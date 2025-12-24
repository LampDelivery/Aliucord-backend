package commands

import (
	"fmt"
	"net/http"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
)

func init() {
	addCommand(&Command{
		CreateCommandData: api.CreateCommandData{
			Name:        "minky",
			Description: "Send a random Minky image",
		},
		Execute: minkyCommand,
	})
}

func minkyCommand(e *gateway.InteractionCreateEvent, _ *discord.CommandInteraction) error {

	if err := s.RespondInteraction(e.ID, e.Token, api.InteractionResponse{
		Type: api.DeferredMessageInteractionWithSource,
		Data: &api.InteractionResponseData{Flags: discord.EphemeralMessage},
	}); err != nil {
		return err
	}

	url := fmt.Sprintf("https://minky.materii.dev?cb=%d", time.Now().Unix())
	resp, err := http.Get(url)
	if err != nil {
		return editReply(e, "❌ Failed to fetch Minky image.")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return editReply(e, fmt.Sprintf("❌ Minky API returned %d", resp.StatusCode))
	}

	_, err = s.EditInteractionResponse(e.AppID, e.Token, api.EditInteractionResponseData{
		Content: option.NewNullableString("Here's a random Minky 🐱"),
		Embeds: &[]discord.Embed{
			{
				Image: &discord.EmbedImage{URL: url},
			},
		},
	})
	if err != nil {
		return editReply(e, "❌ Failed to send Minky image.")
	}
	return nil
}
