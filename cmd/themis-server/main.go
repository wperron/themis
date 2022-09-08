package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"

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
		log.Fatalln("fatal error: failed to touch database file:", err)
	}

	connString := fmt.Sprintf(CONN_STRING_PATTERN, *dbFile)

	store, err = themis.NewStore(connString)
	if err != nil {
		log.Fatalln("fatal error: failed to initialize database:", err)
	}

	authToken, ok := os.LookupEnv("DISCORD_TOKEN")
	if !ok {
		log.Fatalln("fatal error: no auth token found at DISCORD_TOKEN env var")
	}

	appId, ok := os.LookupEnv("DISCORD_APP_ID")
	if !ok {
		log.Fatalln("fatal error: no app id found at DISCORD_TOKEN env var")
	}

	guildId, ok := os.LookupEnv("DISCORD_GUILD_ID")
	if !ok {
		log.Fatalln("fatal error: no guild id found at DISCORD_TOKEN env var")
	}

	discord, err := discordgo.New(fmt.Sprintf("Bot %s", authToken))
	if err != nil {
		log.Fatalln("fatal error: failed to create discord app:", err)
	}

	log.Printf("connected to discord: app_id=%s, guild_id=%s\n", appId, guildId)

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
				log.Println("[error] failed to respond to command:", err)
			}
		},
		"list-claims": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			claims, err := store.ListClaims(ctx)
			if err != nil {
				err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Oops, something went wrong! :(",
					},
				})
				if err != nil {
					log.Println("[error] failed to respond to command:", err)
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
				log.Println("[error] failed to respond to command:", err)
			}
		},
		"claim": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			if i.Type == discordgo.InteractionApplicationCommandAutocomplete {
				handleClaimAutocomplete(ctx, store, s, i)
				return
			}

			opts := i.ApplicationCommandData().Options
			claimType, err := themis.ClaimTypeFromString(opts[0].StringValue())
			if err != nil {
				err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "You can only take claims of types `area`, `region` or `trade`",
					},
				})
				if err != nil {
					log.Println("[error] failed to respond to command:", err)
				}
				return
			}
			name := opts[1].StringValue()

			player := i.Member.Nick
			if player == "" {
				player = i.Member.User.Username
			}

			err = store.Claim(ctx, player, name, claimType)
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
						log.Println("[error] failed to respond to command:", err)
					}
					return
				}
				fmt.Printf("[error]: failed to acquire claim: %s\n", err)
				err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "failed to acquire claim :(",
					},
				})
				if err != nil {
					log.Println("[error] failed to respond to command:", err)
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
				log.Println("[error] failed to respond to command:", err)
			}
		},
	}

	registerHandlers(discord, handlers)

	err = discord.Open()
	if err != nil {
		log.Fatalln("fatal error: failed to open session:", err)
	}
	defer discord.Close()

	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, c := range commands {
		command, err := discord.ApplicationCommandCreate(appId, guildId, c)
		if err != nil {
			log.Fatalln("fatal error: failed to register command:", err)
		}
		registeredCommands[i] = command
	}

	log.Printf("registered %d commands\n", len(registeredCommands))

	go func() {
		if err := serve(":8080"); err != nil {
			log.Printf("[error]: %s\n", err)
		}
		cancel()
	}()

	<-ctx.Done()

	for _, c := range registeredCommands {
		err = discord.ApplicationCommandDelete(appId, guildId, c.ID)
		if err != nil {
			log.Printf("[error]: failed to delete command: %s\n", err)
		}
	}
	log.Println("deregistered commands, bye bye!")
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
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})
	sess.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := handlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
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
		if len(c.Type) > maxLengths[2] {
			maxLengths[2] = len(c.Type)
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
		log.Printf("[error]: %s\n", err)
		return
	}

	availability, err := store.ListAvailability(ctx, claimType, opts[1].StringValue())
	if err != nil {
		log.Printf("[error]: %s\n", err)
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
		log.Printf("[error]: %s\n", err)
	}
}

func serve(address string) error {
	http.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	}))

	return http.ListenAndServe(address, nil)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
