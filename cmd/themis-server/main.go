package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"

	"go.wperron.io/themis"
)

const (
	DB_FILE          = "prod.db"
	CONN_STRING      = "file:" + DB_FILE + "?cache=shared&mode=rw&_journal_mode=WAL"
	DISCORD_APP_ID   = "1014881815921705030"
	DISCORD_GUILD_ID = "1014883118764806164"
)

var (
	dbFile = flag.String("db", "", "SQlite database file path")

	store *themis.Store
)

var ()

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGKILL, syscall.SIGINT)
	defer cancel()

	flag.Parse()

	err := touchDbFile(*dbFile)
	if err != nil {
		log.Fatalln("fatal error: failed to touch database file:", err)
	}

	store, err = themis.NewStore(CONN_STRING)
	if err != nil {
		log.Fatalln("fatal error: failed to initialize database:", err)
	}

	authToken, ok := os.LookupEnv("DISCORD_TOKEN")
	if !ok {
		log.Fatalln("fatal error: no auth token found at DISCORD_TOKEN env var")
	}

	discord, err := discordgo.New(fmt.Sprintf("Bot %s", authToken))
	if err != nil {
		log.Fatalln("fatal error: failed to create discord app:", err)
	}

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "themis",
			Description: "Call dibs on EU4 provinces",
			Type:        discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "ping",
					Description: "Ping Themis",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "list-claims",
					Description: "List current claims",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "claim",
					Description: "Take a claim on provinces",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:        "claim-type",
							Description: "one of `area`, `region` or `trade`",
							Type:        discordgo.ApplicationCommandOptionString,
						},
						{
							Name:        "name",
							Description: "the name of zone claimed",
							Type:        discordgo.ApplicationCommandOptionString,
						},
					},
				},
			},
		},
	}
	handlers := map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"themis": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			options := i.ApplicationCommandData().Options

			switch options[0].Name {
			case "ping":
				err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Pong",
					},
				})
				if err != nil {
					log.Println("[error] failed to respond to command:", err)
				}
			case "list-claims":
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
				for _, c := range claims {
					sb.WriteString(fmt.Sprintf("%s\n", c))
				}

				err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: sb.String(),
					},
				})
				if err != nil {
					log.Println("[error] failed to respond to command:", err)
				}
			case "claim":
				opts := options[0].Options
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
			default:
				err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("Oops, I don't know any `%s` action", options[0].Name),
					},
				})
				if err != nil {
					log.Println("[error] failed to respond to command:", err)
				}
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
		command, err := discord.ApplicationCommandCreate(DISCORD_APP_ID, DISCORD_GUILD_ID, c)
		if err != nil {
			log.Fatalln("fatal error: failed to register command:", err)
		}
		registeredCommands[i] = command
	}

	log.Printf("registered %d commands\n", len(registeredCommands))
	<-ctx.Done()

	for _, c := range registeredCommands {
		err = discord.ApplicationCommandDelete(DISCORD_APP_ID, DISCORD_GUILD_ID, c.ID)
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

func registerHandlers(sess *discordgo.Session, handlers map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate)) {
	sess.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})
	sess.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := handlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})
}
