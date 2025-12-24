package commands

import (
	"fmt"
	"math/rand"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Aliucord/Aliucord-backend/common"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
)

var (
	s             *state.State
	config        *common.BotConfig
	logger        *common.ExtendedLogger
	commandsMap   = make(map[string]*Command)
	commandsCount = 0

	idRegex = regexp.MustCompile("\\d{17,19}")
)

// Pagination state tracked per message for interactive buttons
type listQuery struct {
	Kind   string // "plugins" or "themes"
	Search string
	Author string
	Page   int
	Total  int
}

var paginationState = make(map[discord.MessageID]*listQuery)

func InitCommands(botLogger *common.ExtendedLogger, botConfig *common.BotConfig, state *state.State) {
	s = state
	logger = botLogger
	config = botConfig

	initModCommands()

	logger.Printf("Loaded %d commands\n", commandsCount)

	ready := s.Ready()
	commands := common.MapTransform(commandsMap, func(_ string, command *Command) api.CreateCommandData {
		return command.CreateCommandData
	})
	for _, guild := range ready.Guilds {
		guildID := guild.ID

		_, err := s.BulkOverwriteGuildCommands(ready.Application.ID, guildID, commands)
		if err == nil {
			logger.Printf("Registered commands in %d guild\n", guildID)
		} else {
			logger.Printf("Failed to register commands in %d guild (%v)\n", guildID, err)
		}
	}

	s.AddHandler(func(e *gateway.InteractionCreateEvent) {
		switch d := e.Data.(type) {
		case *discord.CommandInteraction:
			command, ok := commandsMap[d.Name]
			if !ok || command == nil {
				return
			}

			if !slices.Contains(config.OwnerIDs, e.Member.User.ID) &&
				(command.OwnerOnly || command.ModOnly && !slices.Contains(e.Member.RoleIDs, config.RoleIDs.ModRole)) {
				return
			}

			if err := command.Execute(e, d); err != nil {
				content := option.NewNullableString("Something went wrong, sorry :(")
				if _, err2 := s.InteractionResponse(e.AppID, e.Token); err2 == nil {
					_, err2 = s.EditInteractionResponse(e.AppID, e.Token, api.EditInteractionResponseData{
						Content: content,
					})
					logger.LogIfErr(err2)
				} else {
					logger.LogIfErr(s.RespondInteraction(e.ID, e.Token, api.InteractionResponse{
						Type: api.MessageInteractionWithSource,
						Data: &api.InteractionResponseData{
							Content: content,
							Flags:   discord.EphemeralMessage,
						},
					}))
				}

				logger.Printf("Error while running command %s\n%v\n", command.Name, err)
			}
		case *discord.ButtonInteraction:

			if d.CustomID == "" || e.Message == nil {
				return
			}
			parts := strings.Split(string(d.CustomID), ":")
			if len(parts) != 3 || parts[0] != "page" {
				return
			}
			kind := parts[1]
			action := parts[2]
			st, ok := paginationState[e.Message.ID]
			if !ok || st == nil {
				return
			}

			switch action {
			case "prev":
				if st.Page > 1 {
					st.Page--
				}
			case "next":
				if st.Page < st.Total {
					st.Page++
				}
			default:
				return
			}

			var content string
			var total int
			var comps discord.ContainerComponents
			var err error

			switch kind {
			case "plugins":
				content, total, comps, err = renderPluginsPage(st.Search, st.Author, st.Page)
			case "themes":
				content, total, comps, err = renderThemesPage(st.Search, st.Author, st.Page)
			default:
				return
			}

			if err != nil {

				_ = s.RespondInteraction(e.ID, e.Token, api.InteractionResponse{
					Type: api.UpdateMessage,
					Data: &api.InteractionResponseData{
						Content: option.NewNullableString("Something went wrong while paging."),
					},
				})
				return
			}

			st.Total = total

			_ = s.RespondInteraction(e.ID, e.Token, api.InteractionResponse{
				Type: api.UpdateMessage,
				Data: &api.InteractionResponseData{
					Content:         option.NewNullableString(content),
					Components:      &comps,
					AllowedMentions: &api.AllowedMentions{},
				},
			})
		}
	})

	s.AddHandler(func(msg *gateway.MessageCreateEvent) {
		if msg.Author.Bot || msg.Member == nil {
			return
		}
		content := strings.TrimSpace(msg.Content)
		if !strings.HasPrefix(content, "!") {
			return
		}
		content = strings.TrimPrefix(content, "!")
		fields := strings.Fields(content)
		if len(fields) == 0 {
			return
		}
		name := strings.ToLower(fields[0])
		args := parseKVArgs(fields[1:])

		switch name {
		case "plugins":
			search := strings.TrimSpace(args["search"])
			author := strings.TrimSpace(args["author"])
			page := 1
			if p := strings.TrimSpace(args["page"]); p != "" {
				if v, err := strconv.Atoi(p); err == nil && v > 0 {
					page = v
				}
			}
			content, total, comps, err := renderPluginsPage(search, author, page)
			if err != nil {
				logger.LogWithCtxIfErr("prefix plugins", err)
				return
			}
			sent, err := s.SendMessageComplex(msg.ChannelID, api.SendMessageData{
				Content:         content,
				Components:      comps,
				AllowedMentions: &api.AllowedMentions{RepliedUser: option.False},
				Reference:       &discord.MessageReference{MessageID: msg.ID},
			})
			if err == nil && sent != nil {
				paginationState[sent.ID] = &listQuery{Kind: "plugins", Search: search, Author: author, Page: page, Total: total}
			}

		case "themes":
			search := strings.TrimSpace(args["search"])
			author := strings.TrimSpace(args["author"])
			page := 1
			if p := strings.TrimSpace(args["page"]); p != "" {
				if v, err := strconv.Atoi(p); err == nil && v > 0 {
					page = v
				}
			}
			content, total, comps, err := renderThemesPage(search, author, page)
			if err != nil {
				logger.LogWithCtxIfErr("prefix themes", err)
				return
			}
			sent, err := s.SendMessageComplex(msg.ChannelID, api.SendMessageData{
				Content:         content,
				Components:      comps,
				AllowedMentions: &api.AllowedMentions{RepliedUser: option.False},
				Reference:       &discord.MessageReference{MessageID: msg.ID},
			})
			if err == nil && sent != nil {
				paginationState[sent.ID] = &listQuery{Kind: "themes", Search: search, Author: author, Page: page, Total: total}
			}

		case "random-plugin":
			list, err := fetchPlugins()
			if err != nil || len(list) == 0 {
				_, _ = s.SendMessageComplex(msg.ChannelID, api.SendMessageData{
					Content:         "❌ Failed to fetch plugins.",
					AllowedMentions: &api.AllowedMentions{RepliedUser: option.False},
					Reference:       &discord.MessageReference{MessageID: msg.ID},
				})
				return
			}
			rand.Seed(time.Now().UnixNano())
			p := list[rand.Intn(len(list))]
			c := "**Random Plugin Suggestion**\n\n" + formatPluginLine(p) + "\n\n-# hold this message (not the links) to install"
			_, _ = s.SendMessageComplex(msg.ChannelID, api.SendMessageData{
				Content:         c,
				AllowedMentions: &api.AllowedMentions{RepliedUser: option.False},
				Reference:       &discord.MessageReference{MessageID: msg.ID},
			})

		case "minky":
			url := fmt.Sprintf("https://minky.materii.dev?cb=%d", time.Now().Unix())
			_, _ = s.SendMessageComplex(msg.ChannelID, api.SendMessageData{
				Content:         "Here's a random Minky 🐱",
				Embeds:          []discord.Embed{{Image: &discord.EmbedImage{URL: url}}},
				AllowedMentions: &api.AllowedMentions{RepliedUser: option.False},
				Reference:       &discord.MessageReference{MessageID: msg.ID},
			})
		}
	})
}

// parseKVArgs parses tokens like key=value into a map
func parseKVArgs(tokens []string) map[string]string {
	m := make(map[string]string, len(tokens))
	for _, t := range tokens {
		if eq := strings.IndexByte(t, '='); eq > 0 {
			k := strings.ToLower(strings.TrimSpace(t[:eq]))
			v := strings.TrimSpace(t[eq+1:])
			m[k] = v
		}
	}
	return m
}

type Command struct {
	api.CreateCommandData

	ModOnly   bool
	OwnerOnly bool
	Execute   func(e *gateway.InteractionCreateEvent, d *discord.CommandInteraction) error
}

func getMultipleUsersOption(d *discord.CommandInteraction) []discord.UserID {
	usersOption := findOption(d, "users")
	if usersOption == nil {
		return []discord.UserID{}
	}

	ids := idRegex.FindAllString(usersOption.String(), -1)
	return common.SliceTransform(ids, func(idStr string) discord.UserID {
		id, _ := strconv.ParseUint(idStr, 10, 64)
		return discord.UserID(id)
	})
}

func getUserOrUsersOption(d *discord.CommandInteraction) []discord.UserID {
	userIDs := getMultipleUsersOption(d)
	if users := d.Resolved.Users; len(users) > 0 {
		userIDs = append(userIDs, common.MapKeys(users)...)
	}
	return userIDs
}

func reply(e *gateway.InteractionCreateEvent, content string) error {
	return replyWithFlags(e, 0, content, nil)
}

func ephemeralReply(e *gateway.InteractionCreateEvent, content string) error {
	return replyWithFlags(e, discord.EphemeralMessage, content, nil)
}

func replyErr(e *gateway.InteractionCreateEvent, context string, err error) error {
	logger.Println("Err while " + context)
	logger.Println(err)
	return ephemeralReply(e, "Something went wrong: ```\n"+err.Error()+"```")
}

func replyWithFlags(e *gateway.InteractionCreateEvent, flags discord.MessageFlags, content string, embeds *[]discord.Embed) error {
	return s.RespondInteraction(e.ID, e.Token, api.InteractionResponse{
		Type: api.MessageInteractionWithSource,
		Data: &api.InteractionResponseData{
			Content:         option.NewNullableString(content),
			Flags:           flags,
			Embeds:          embeds,
			AllowedMentions: &api.AllowedMentions{},
		},
	})
}

func editReply(e *gateway.InteractionCreateEvent, content string) error {
	_, err := s.EditInteractionResponse(e.AppID, e.Token, api.EditInteractionResponseData{
		Content: option.NewNullableString(content),
	})
	return err
}

func findOption(d *discord.CommandInteraction, name string) *discord.CommandInteractionOption {
	return findOption2(d.Options, name)
}

func findOption2(opts discord.CommandInteractionOptions, name string) *discord.CommandInteractionOption {
	return common.Find(opts, func(option *discord.CommandInteractionOption) bool {
		return option.Name == name
	})
}

func boolOrDefault(d *discord.CommandInteraction, name string, def bool) bool {
	boolOption := findOption(d, name)
	if boolOption == nil {
		return def
	}
	ret, err := boolOption.BoolValue()
	return common.Ternary(err == nil, ret, def)
}

func addCommand(cmd *Command) {
	commandsCount++
	commandsMap[cmd.Name] = cmd
}
