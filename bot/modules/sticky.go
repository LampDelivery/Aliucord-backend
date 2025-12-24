package modules

import (
	"strings"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

type StickyConfig struct {
	Text           string
	Cooldown       time.Duration
	IncludeWarning bool

	LastSent     time.Time
	LastStickyID discord.MessageID
}

var stickyByChannel = map[discord.ChannelID]*StickyConfig{}

func init() {
	modules = append(modules, initStickyModule)
}

func initStickyModule() {
	s.AddHandler(func(msg *gateway.MessageCreateEvent) {
		if msg.Author.Bot || msg.Member == nil {
			return
		}
		cfg, ok := stickyByChannel[msg.ChannelID]
		if !ok || cfg == nil {
			return
		}
		if time.Since(cfg.LastSent) < cfg.Cooldown {
			return
		}

		content := cfg.Text
		if cfg.IncludeWarning {
			content = "**__stickied message:__**\n" + content
		}

		sent, err := s.SendMessageComplex(msg.ChannelID, api.SendMessageData{
			Content:         content,
			AllowedMentions: &api.AllowedMentions{},
		})
		if err == nil && sent != nil {

			if cfg.LastStickyID.IsValid() {
				_ = s.DeleteMessage(msg.ChannelID, cfg.LastStickyID, api.AuditLogReason(""))
			}
			cfg.LastStickyID = sent.ID
			cfg.LastSent = time.Now()
		}
	})
}

// SetSticky configures or updates the sticky in the given channel.
func SetSticky(channelID discord.ChannelID, text string, cooldownSeconds int, includeWarning bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if cooldownSeconds < 0 {
		cooldownSeconds = 0
	}
	stickyByChannel[channelID] = &StickyConfig{
		Text:           text,
		Cooldown:       time.Duration(cooldownSeconds) * time.Second,
		IncludeWarning: includeWarning,
		LastSent:       time.Time{},
		LastStickyID:   0,
	}
}

// ClearSticky removes sticky from the channel and deletes the last sticky message if present.
func ClearSticky(channelID discord.ChannelID) {
	if cfg, ok := stickyByChannel[channelID]; ok {
		if cfg.LastStickyID.IsValid() {
			_ = s.DeleteMessage(channelID, cfg.LastStickyID, api.AuditLogReason(""))
		}
		delete(stickyByChannel, channelID)
	}
}
