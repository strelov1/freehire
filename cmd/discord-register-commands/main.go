// Command discord-register-commands registers the bot's guild slash commands
// (/link, /contribute) with Discord. Run it once after deploying the bot or
// whenever a command's shape changes — Discord caches guild command
// definitions server-side, so the running server never needs to re-register
// them itself. Unlike the cron workers under cmd/, this binary touches no
// database and does not use worker.Bootstrap.
package main

import (
	"context"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/discordbot"
)

func main() {
	botToken := os.Getenv("DISCORD_BOT_TOKEN")
	applicationID := os.Getenv("DISCORD_APPLICATION_ID")
	guildID := os.Getenv("DISCORD_GUILD_ID")
	if botToken == "" || applicationID == "" || guildID == "" {
		log.Fatal("DISCORD_BOT_TOKEN, DISCORD_APPLICATION_ID, and DISCORD_GUILD_ID are all required")
	}

	commands := []discordbot.Command{
		{
			Name:        "link",
			Description: "Link your freehire account to Discord",
			Options: []discordbot.CommandOption{
				{
					Type:        discordbot.CommandOptionTypeString,
					Name:        "token",
					Description: "The link token from freehire.me",
					Required:    true,
				},
			},
		},
		{
			Name:        "contribute",
			Description: "Contribute a job posting URL to freehire",
			Options: []discordbot.CommandOption{
				{
					Type:        discordbot.CommandOptionTypeString,
					Name:        "url",
					Description: "The job posting URL",
					Required:    true,
				},
			},
		},
	}

	client := discordbot.NewClient(botToken)
	if err := client.RegisterCommands(context.Background(), applicationID, guildID, commands); err != nil {
		log.Fatalf("register commands: %v", err)
	}
	log.Print("registered /link and /contribute commands")
}
