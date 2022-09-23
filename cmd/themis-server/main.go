package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"

	"go.wperron.io/themis"
)

const (
	CONN_STRING_PATTERN = "file:%s?cache=shared&mode=rw&_journal_mode=WAL"
)

var (
	dbFile = flag.String("db", "", "SQlite database file path")

	store *themis.Store
)

type Handler func(s *discordgo.Session, i *discordgo.InteractionCreate)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGKILL, syscall.SIGINT)
	defer cancel()

	flag.Parse()

	err := touchDbFile(*dbFile)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to touch database file")
	}

	connString := fmt.Sprintf(CONN_STRING_PATTERN, *dbFile)

	store, err = themis.NewStore(connString)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize database")
	}
	defer store.Close()

	authToken, ok := os.LookupEnv("DISCORD_TOKEN")
	if !ok {
		log.Fatal().Err(err).Msg("no auth token found at DISCORD_TOKEN env var")
	}

	appId, ok := os.LookupEnv("DISCORD_APP_ID")
	if !ok {
		log.Fatal().Err(err).Msg("no app id found at DISCORD_APP_ID env var")
	}

	guildId, ok := os.LookupEnv("DISCORD_GUILD_ID")
	if !ok {
		log.Fatal().Err(err).Msg("no guild id found at DISCORD_GUILD_ID env var")
	}

	discord, err := discordgo.New(fmt.Sprintf("Bot %s", authToken))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize discord session")
	}

	log.Info().Str("app_id", appId).Str("guild_id", guildId).Msg("connected to discord")

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "ping",
			Description: "Ping Themis",
			Type:        discordgo.ChatApplicationCommand,
		},
		{
			Name:        "list-claims",
			Description: "List current claims",
			Type:        discordgo.ChatApplicationCommand,
		},
		{
			Name:        "claim",
			Description: "Take a claim on provinces",
			Type:        discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "claim-type",
					Description: "one of `area`, `region` or `trade`",
					Type:        discordgo.ApplicationCommandOptionString,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Area", Value: themis.CLAIM_TYPE_AREA},
						{Name: "Region", Value: themis.CLAIM_TYPE_REGION},
						{Name: "Trade Node", Value: themis.CLAIM_TYPE_TRADE},
					},
				},
				{
					Name:         "name",
					Description:  "the name of zone claimed",
					Type:         discordgo.ApplicationCommandOptionString,
					Autocomplete: true,
				},
			},
		},
		{
			Name:        "describe-claim",
			Description: "Get details on a claim",
			Type:        discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "id",
					Description: "Numerical ID for the claim",
					Type:        discordgo.ApplicationCommandOptionInteger,
				},
			},
		},
		{
			Name:        "delete-claim",
			Description: "Release one of your claims",
			Type:        discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "id",
					Description: "numerical ID for the claim",
					Type:        discordgo.ApplicationCommandOptionInteger,
				},
			},
		},
		{
			Name:        "flush",
			Description: "Remove all claims from the database and prepare for the next game!",
			Type:        discordgo.ChatApplicationCommand,
		},
		{
			Name:        "query",
			Description: "Run a raw SQL query on the database",
			Type:        discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "query",
					Description: "Raw SQL query",
					Type:        discordgo.ApplicationCommandOptionString,
				},
			},
		},
	}
	handlers := map[string]Handler{
		"ping": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Pong",
				},
			})
			if err != nil {
				log.Error().Err(err).Msg("failed to respond to interaction")
			}
		},
		"list-claims": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			claims, err := store.ListClaims(ctx)
			if err != nil {
				log.Error().Err(err).Msg("failed to list claims")
				err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Oops, something went wrong! :(",
					},
				})
				if err != nil {
					log.Error().Err(err).Msg("failed to respond to interaction")
				}
			}

			sb := strings.Builder{}
			sb.WriteString(fmt.Sprintf("There are currently %d claims:\n", len(claims)))
			sb.WriteString("```\n")
			sb.WriteString(formatClaimsTable(claims))
			sb.WriteString("```\n")

			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: sb.String(),
				},
			})
			if err != nil {
				log.Error().Err(err).Msg("failed to respond to interaction")
			}
		},
		"claim": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			if i.Type == discordgo.InteractionApplicationCommandAutocomplete {
				handleClaimAutocomplete(ctx, store, s, i)
				return
			}

			opts := i.ApplicationCommandData().Options
			if len(opts) != 2 {
				err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "`claim-type` and `name` are mandatory parameters",
					},
				})
				if err != nil {
					log.Error().Err(err).Msg("failed to respond to interaction")
				}
				return
			}

			claimType, err := themis.ClaimTypeFromString(opts[0].StringValue())
			if err != nil {
				log.Error().Err(err).Str("claim_type", opts[0].StringValue()).Msg("failed to parse claim")
				err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "You can only take claims of types `area`, `region` or `trade`",
					},
				})
				if err != nil {
					log.Error().Err(err).Msg("failed to respond to interaction")
				}
				return
			}
			name := opts[1].StringValue()

			player := i.Member.Nick
			if player == "" {
				player = i.Member.User.Username
			}

			userId := i.Member.User.ID

			_, err = store.Claim(ctx, userId, player, name, claimType)
			if err != nil {
				conflict, ok := err.(themis.ErrConflict)
				if ok {
					sb := strings.Builder{}
					sb.WriteString("Some provinces are already claimed:\n```\n")
					for _, c := range conflict.Conflicts {
						sb.WriteString(fmt.Sprintf("  - %s\n", c))
					}
					sb.WriteString("```\n")

					err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: sb.String(),
						},
					})
					if err != nil {
						log.Error().Err(err).Msg("failed to respond to interaction")
					}
					return
				}

				log.Error().Err(err).Msg("failed to acquire claim")
				err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "failed to acquire claim :(",
					},
				})
				if err != nil {
					log.Error().Err(err).Msg("failed to respond to interaction")
				}
				return
			}

			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("Claimed %s for %s!", name, player),
				},
			})
			if err != nil {
				log.Error().Err(err).Msg("failed to respond to interaction")
			}
		},
		"describe-claim": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			id := i.ApplicationCommandData().Options[0]
			detail, err := store.DescribeClaim(ctx, int(id.IntValue()))
			if err != nil {
				log.Error().Err(err).Msg("failed to describe claim")
				err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "woops, something went wrong :(",
					},
				})
				if err != nil {
					log.Error().Err(err).Msg("failed to respond to interaction")
				}
			}

			sb := strings.Builder{}
			sb.WriteString(fmt.Sprintf("#%d %s %s (%s)\n", detail.ID, detail.Name, detail.Type, detail.Player))
			for _, p := range detail.Provinces {
				sb.WriteString(fmt.Sprintf(" - %s\n", p))
			}

			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: sb.String(),
				},
			})
			if err != nil {
				log.Error().Err(err).Msg("failed to respond to interaction")
			}
		},
		"delete-claim": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			id := i.ApplicationCommandData().Options[0]
			userId := i.Member.User.ID
			err := store.DeleteClaim(ctx, int(id.IntValue()), userId)
			if err != nil {
				msg := "Oops, something went wrong :( blame @wperron"
				if errors.Is(err, themis.ErrNoSuchClaim) {
					msg = fmt.Sprintf("Claim #%d not found for %s", id.IntValue(), i.Member.Nick)
				}
				log.Error().Err(err).Msg("failed to delete claim")
				err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: msg,
					},
				})
				if err != nil {
					log.Error().Err(err).Msg("failed to respond to interaction")
				}
			}

			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Got it chief.",
				},
			})
			if err != nil {
				log.Error().Err(err).Msg("failed to respond to interaction")
			}
		},
		"flush": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseModal,
				Data: &discordgo.InteractionResponseData{
					CustomID: "modals_flush_" + i.Interaction.Member.User.ID,
					Title:    "Are you sure?",
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{
								discordgo.TextInput{
									CustomID:    "confirmation",
									Label:       "Delete all claims permanently? [y/N]",
									Style:       discordgo.TextInputShort,
									Placeholder: "",
									Value:       "",
									Required:    true,
									MinLength:   1,
									MaxLength:   45,
								},
							},
						},
					},
				},
			}); err != nil {
				log.Error().Err(err).Msg("failed to respond to interaction")
			}
		},
		"query": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			roDB, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?cache=private&mode=ro", *dbFile))
			if err != nil {
				log.Error().Err(err).Msg("failed to open read-only copy of databse")
			}

			q := i.ApplicationCommandData().Options[0].StringValue()
			deadlined, cancelDeadline := context.WithTimeout(ctx, 15*time.Second)
			defer cancelDeadline()
			rows, err := roDB.QueryContext(deadlined, q)
			if err != nil {
				log.Error().Err(err).Msg("failed to exec user-provided query")
				return
			}

			fmtd, err := themis.FormatRows(rows)
			if err != nil {
				log.Error().Err(err).Msg("failed to format rows")
			}

			// 2000 is a magic number here, it's the character limit for a discord
			// message, we're cutting slightly under that to allow the backticks
			// for the monospaced block.
			table := fmt.Sprintf("```\n%s\n```", fmtd[:min(len(fmtd), 1990)])

			if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: table,
				},
			}); err != nil {
				log.Error().Err(err).Msg("failed to respond to interaction")
			}
		},
	}

	registerHandlers(discord, handlers)

	err = discord.Open()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open discord websocket")
	}
	defer discord.Close()

	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, c := range commands {
		command, err := discord.ApplicationCommandCreate(appId, guildId, c)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to register command")
		}
		registeredCommands[i] = command
	}

	log.Info().Int("count", len(registeredCommands)).Msg("registered commands")

	go func() {
		if err := serve(":8080"); err != nil {
			log.Error().Err(err).Msg("failed to serve requests")
		}
		cancel()
	}()

	<-ctx.Done()
	log.Info().Msg("context cancelled, exiting")

	for _, c := range registeredCommands {
		err = discord.ApplicationCommandDelete(appId, guildId, c.ID)
		if err != nil {
			log.Error().Err(err).Msg("failed to deregister commands")
		}
	}
	log.Info().Msg("deregistered commands, exiting")
	os.Exit(0)
}

func touchDbFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			f.Close()
		} else {
			return err
		}
	}
	f.Close()

	return nil
}

func registerHandlers(sess *discordgo.Session, handlers map[string]Handler) {
	sess.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Info().Str("user_id", fmt.Sprintf("%s#%s", s.State.User.Username, s.State.User.Discriminator)).Msg("logged in")
	})
	sess.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if h, ok := handlers[i.ApplicationCommandData().Name]; ok {
				h(s, i)
			}
		case discordgo.InteractionModalSubmit:
			if strings.HasPrefix(i.ModalSubmitData().CustomID, "modals_flush_") {
				sub := i.ModalSubmitData().Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
				sub = strings.ToLower(sub)
				if sub == "y" || sub == "ye" || sub == "yes" {
					err := store.Flush(context.Background())
					msg := "Flushed all claims!"
					if err != nil {
						log.Error().Err(err).Msg("failed to flush claims")
						msg = "failed to flush claims from database"
					}

					err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: msg,
						},
					})
					if err != nil {
						log.Error().Err(err).Msg("failed to respond to interaction")
					}
					return
				}

				err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Aborted...",
					},
				})
				if err != nil {
					log.Error().Err(err).Msg("failed to respond to interaction")
				}
				return
			}
		}
	})
}

const TABLE_PATTERN = "| %-*s | %-*s | %-*s | %-*s |\n"

func formatClaimsTable(claims []themis.Claim) string {
	sb := strings.Builder{}
	maxLengths := []int{2, 6, 4, 4} // id, player, type, name
	for _, c := range claims {
		sid := strconv.Itoa(c.ID)
		if len(sid) > maxLengths[0] {
			maxLengths[0] = len(sid)
		}
		if len(c.Player) > maxLengths[1] {
			maxLengths[1] = len(c.Player)
		}
		// The raw claim value is different from the formatted string
		strType := c.Type.String()
		if len(strType) > maxLengths[2] {
			maxLengths[2] = len(strType)
		}
		if len(c.Name) > maxLengths[3] {
			maxLengths[3] = len(c.Name)
		}
	}

	sb.WriteString(fmt.Sprintf(TABLE_PATTERN, maxLengths[0], "ID", maxLengths[1], "Player", maxLengths[2], "Type", maxLengths[3], "Name"))
	sb.WriteString(fmt.Sprintf(TABLE_PATTERN, maxLengths[0], strings.Repeat("-", maxLengths[0]), maxLengths[1], strings.Repeat("-", maxLengths[1]), maxLengths[2], strings.Repeat("-", maxLengths[2]), maxLengths[3], strings.Repeat("-", maxLengths[3])))
	for _, c := range claims {
		sb.WriteString(fmt.Sprintf(TABLE_PATTERN, maxLengths[0], strconv.Itoa(c.ID), maxLengths[1], c.Player, maxLengths[2], c.Type, maxLengths[3], c.Name))
	}
	return sb.String()
}

func handleClaimAutocomplete(ctx context.Context, store *themis.Store, s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := i.ApplicationCommandData().Options
	claimType, err := themis.ClaimTypeFromString(opts[0].StringValue())
	if err != nil {
		log.Error().Err(err).Msg("failed to parse claim type")
		return
	}

	availability, err := store.ListAvailability(ctx, claimType, opts[1].StringValue())
	if err != nil {
		log.Error().Err(err).Msg("failed to list availabilities")
		return
	}

	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0, len(availability))
	for _, s := range availability {
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  s,
			Value: s,
		})
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{
			Choices: choices[:min(len(choices), 25)],
		},
	}); err != nil {
		log.Error().Err(err).Msg("failed to respond to interaction")
	}
}

func serve(address string) error {
	http.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK")) //nolint:errcheck // this is expected to always work, 'trust me bro' guaranteed
	}))

	return http.ListenAndServe(address, nil)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
