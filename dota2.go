package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/faceit/go-steam"
	"github.com/faceit/go-steam/gsbot"
	"github.com/faceit/go-steam/protocol/steamlang"
	"github.com/joho/godotenv"
	"github.com/paralin/go-dota2"
	devents "github.com/paralin/go-dota2/events"
	log "github.com/sirupsen/logrus"
)

type PlayerProfile struct {
	RankTier        int
	LeaderboardRank int
}

func GetAllProfileCards(players []uint32) map[uint32]PlayerProfile {
	responses := make(map[uint32]PlayerProfile)
	le := log.New()
	le.SetLevel(log.DebugLevel)
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	fmt.Println("Starting")
	steamUser := os.Getenv("STEAM_USERNAME")
	steamPass := os.Getenv("STEAM_PASSWORD")
	loginInfo := new(gsbot.LogOnDetails)
	loginInfo.Username = steamUser
	loginInfo.Password = steamPass

	bot := gsbot.Default()
	client := bot.Client
	auth := gsbot.NewAuth(bot, loginInfo, "sentry.bin")
	serverList := gsbot.NewServerList(bot, "serverlist.json")
	serverList.Connect()
	d2 := dota2.New(client, le)
	defer client.Disconnect()
	defer d2.Close()
	hello_done := make(chan struct{})

	for event := range client.Events() {
		auth.HandleEvent(event)
		serverList.HandleEvent(event)
		switch e := event.(type) {
		case error:
			fmt.Printf("Error: %v", e)
		case *steam.LoggedOnEvent:
			client.Social.SetPersonaState(steamlang.EPersonaState_Online)
			d2.SetPlaying(true)
			go establishDotaHello(d2, hello_done)
		case *devents.ClientWelcomed:
			hello_done <- struct{}{}
			for _, player := range players {
				fmt.Printf("Requesting player %v\n", player)
				msg, err := d2.GetProfileCard(context.TODO(), player)
				fmt.Println(msg)
				if err != nil {
					le.Error(err)
					continue
				}
				pp := PlayerProfile{}
				if msg.RankTier != nil {
					pp.RankTier = int(*msg.RankTier)
				}
				if msg.LeaderboardRank != nil {
					pp.LeaderboardRank = int(*msg.LeaderboardRank)
				}
				responses[player] = pp
			}
			return responses
		}
	}
	return responses
}

func establishDotaHello(d *dota2.Dota2, done chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			d.SayHello()
		}
	}
}
