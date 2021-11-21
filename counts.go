package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

type CountInfo struct {
	Counts    CountResponse
	AccountID uint32
	Error     bool
}

type CountResponse struct {
	Region *Region `json:"region,omitempty"`
}

type Region struct {
	SEA RegionScore `json:"5,omitempty"`
	AU  RegionScore `json:"7,omitempty"`
}

type RegionScore struct {
	Games int `json:"games"`
	Wins  int `json:"win"`
}

func getPlayerCounts(players *PlayerFile, c *RLHTTPClient) {
	ch := make(chan CountInfo)
	for _, p := range players.Players {
		go playerCountRequest(p, ch, c)
	}
	for range players.Players {
		cinfo := <-ch
		if !cinfo.Error {
			player := players.Players[fmt.Sprint(cinfo.AccountID)]
			player.Counts = &cinfo.Counts
			players.Players[fmt.Sprint(cinfo.AccountID)] = player
		}
	}
}

func playerCountRequest(player Player, ch chan CountInfo, c *RLHTTPClient) {
	url := fmt.Sprintf("https://api.opendota.com/api/players/%v/counts?lobby_type=7&date=7", player.AccountID)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := c.Do(req)
	if err != nil {
		log.Fatalln(err)
	}
	count_info := CountInfo{AccountID: player.AccountID}
	if resp.StatusCode != 200 {
		fmt.Printf("Unexpected response %d\n", resp.StatusCode)
		count_info.Error = true
		ch <- count_info
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	count_resp := CountResponse{}
	json.Unmarshal(body, &count_resp)
	count_info.Counts = count_resp
	ch <- count_info
}
