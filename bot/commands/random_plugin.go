package commands

import (
    "math/rand"
    "time"

    "github.com/diamondburned/arikawa/v3/api"
    "github.com/diamondburned/arikawa/v3/discord"
    "github.com/diamondburned/arikawa/v3/gateway"
)

func init() {
    addCommand(&Command{
        CreateCommandData: api.CreateCommandData{
            Name:        "random-plugin",
            Description: "Get a random Aliucord plugin suggestion",
        },
        Execute: randomPluginCommand,
    })
}

func randomPluginCommand(e *gateway.InteractionCreateEvent, _ *discord.CommandInteraction) error {
    // Ephemeral reply by default
    if err := s.RespondInteraction(e.ID, e.Token, api.InteractionResponse{
        Type: api.DeferredMessageInteractionWithSource,
        Data: &api.InteractionResponseData{Flags: discord.EphemeralMessage},
    }); err != nil {
        return err
    }

    list, err := fetchPlugins()
    if err != nil || len(list) == 0 {
        return editReply(e, "❌ Failed to fetch plugins.")
    }

    rand.Seed(time.Now().UnixNano())
    p := list[rand.Intn(len(list))]
    content := "**Random Plugin Suggestion**\n\n" + formatPluginLine(p) + "\n\n-# hold this message (not the links) to install"
    return editReply(e, content)
}
