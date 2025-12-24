package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/Aliucord/Aliucord-backend/bot"
	"github.com/Aliucord/Aliucord-backend/bot/commands"
	"github.com/Aliucord/Aliucord-backend/bot/modules"
	"github.com/Aliucord/Aliucord-backend/common"
	"github.com/Aliucord/Aliucord-backend/database"
	"github.com/valyala/fasthttp"
)

type fastHttpLogger struct{}

func (*fastHttpLogger) Printf(string, ...interface{}) {}

func main() {
	f, err := os.Open("config.json")
	if err != nil {
		log.Fatal(err)
	}
	var config *common.Config
	err = json.NewDecoder(f).Decode(&config)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}

	token := os.Getenv("DISCORD_BOT_TOKEN")
	if token != "" {
		config.Bot.Token = token
	}

	database.InitDB(config.Database)

	if config.Bot.Enabled && config.Bot.Token != "" {
		modules.UpdateScamTitles()
		bot.StartBot(config)
		defer bot.StopBot()
	}

	log.Println("Starting http server at port", config.Port)
	server := fasthttp.Server{
		Logger: &fastHttpLogger{},
		Handler: func(ctx *fasthttp.RequestCtx) {
			path := string(ctx.Path())
			if strings.HasPrefix(path, "/badges/users/") {
				id := strings.TrimPrefix(path, "/badges/users/")
				if data, ok := commands.Badges.Users[id]; ok {
					json.NewEncoder(ctx).Encode(&data)
					return
				}
			} else if strings.HasPrefix(path, "/badges/guilds/") {
				id := strings.TrimPrefix(path, "/badges/guilds/")
				if data, ok := commands.Badges.Guilds[id]; ok {
					json.NewEncoder(ctx).Encode(data)
					return
				}
			}
			ctx.WriteString("Bot is running!")
		},
	}
	server.ListenAndServe(":" + config.Port)
}
