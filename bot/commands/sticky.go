package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Aliucord/Aliucord-backend/bot/modules"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
)

func init() {
	addCommand(&Command{
		CreateCommandData: api.CreateCommandData{
			Name:        "sticky",
			Description: "Manage stickied messages",
			Options: []discord.CommandOption{
				&discord.SubcommandOption{
					OptionName:  "add",
					Description: "Add or update a stickied message in a channel",
					Options: []discord.CommandOptionValue{
						&discord.StringOption{OptionName: "message", Description: "Message content", Required: true},
						&discord.IntegerOption{OptionName: "cooldown", Description: "Cooldown in seconds (default 120)", Min: option.NewInt(0)},
						&discord.BooleanOption{OptionName: "warning", Description: "Include '**__Stickied Message:__**' header (default on)"},
						&discord.ChannelOption{OptionName: "channel", Description: "Target channel (default: current)"},
					},
				},
				&discord.SubcommandOption{
					OptionName:  "remove",
					Description: "Remove the stickied message from a channel",
					Options: []discord.CommandOptionValue{
						&discord.ChannelOption{OptionName: "channel", Description: "Target channel (default: current)"},
					},
				},
			},
		},
		Execute: stickyCommand,
	})
}

func stickyCommand(e *gateway.InteractionCreateEvent, d *discord.CommandInteraction) error {

	perms, err := s.Permissions(e.ChannelID, e.Member.User.ID)
	if err != nil {
		return ephemeralReply(e, "Could not check your permissions")
	}

	hasModPerms := perms.Has(discord.PermissionManageMessages) ||
		perms.Has(discord.PermissionManageGuild) ||
		perms.Has(discord.PermissionBanMembers) ||
		perms.Has(discord.PermissionKickMembers)

	if !hasModPerms {
		return ephemeralReply(e, "You need moderation permissions to use this command")
	}

	if len(d.Options) == 0 {
		return ephemeralReply(e, "Use subcommands: add/remove")
	}
	sub := d.Options[0]
	switch sub.Name {
	case "add":
		var (
			msgText        string
			cooldown       int64 = 120
			includeWarning       = true
			channelID            = e.ChannelID
		)

		for _, opt := range sub.Options {
			switch opt.Name {
			case "message":
				msgText = strings.TrimSpace(opt.String())
			case "cooldown":
				if v, err := opt.IntValue(); err == nil {
					cooldown = v
				}
			case "warning":
				if v, err := opt.BoolValue(); err == nil {
					includeWarning = v
				}
			case "channel":

				raw := strings.Trim(string(opt.Value), "\"")
				if u, err := strconv.ParseUint(raw, 10, 64); err == nil {
					channelID = discord.ChannelID(u)
				} else if len(d.Resolved.Channels) == 1 {
					for id := range d.Resolved.Channels {
						channelID = id
					}
				}
			}
		}

		if msgText == "" {
			return ephemeralReply(e, "Message content cannot be empty")
		}

		modules.SetSticky(channelID, msgText, int(cooldown), includeWarning)
		return ephemeralReply(e, fmt.Sprintf("Stickied message set in <#%s> (cooldown %ds)", channelID, cooldown))

	case "remove":
		channelID := e.ChannelID
		for _, opt := range sub.Options {
			if opt.Name == "channel" {
				raw := strings.Trim(string(opt.Value), "\"")
				if u, err := strconv.ParseUint(raw, 10, 64); err == nil {
					channelID = discord.ChannelID(u)
				} else if len(d.Resolved.Channels) == 1 {
					for id := range d.Resolved.Channels {
						channelID = id
					}
				}
			}
		}
		modules.ClearSticky(channelID)
		return ephemeralReply(e, fmt.Sprintf("Removed stickied message from <#%s>", channelID))
	}

	return s.RespondInteraction(e.ID, e.Token, api.InteractionResponse{
		Type: api.MessageInteractionWithSource,
		Data: &api.InteractionResponseData{Content: option.NewNullableString("Unknown subcommand"), Flags: discord.EphemeralMessage},
	})
}
