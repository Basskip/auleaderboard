package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"text/template"
	"time"

	"github.com/mazznoer/colorgrad"
	"golang.org/x/time/rate"
)

//RLHTTPClient Rate Limited HTTP Client
type RLHTTPClient struct {
	client      *http.Client
	Ratelimiter *rate.Limiter
}

//Do dispatches the HTTP request to the network
func (c *RLHTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Comment out the below 5 lines to turn off ratelimiting
	ctx := context.Background()
	err := c.Ratelimiter.Wait(ctx) // This is a blocking call. Honors the rate limit
	if err != nil {
		return nil, err
	}
	fmt.Printf("Requesting %v\n", req.URL)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

//NewClient return http client with a ratelimiter
func NewClient(rl *rate.Limiter) *RLHTTPClient {
	c := &RLHTTPClient{
		client:      http.DefaultClient,
		Ratelimiter: rl,
	}
	return c
}

type Player struct {
	AccountID       uint32         `json:"account_id"`
	PersonaName     string         `json:"personaname,omitempty"`
	OverrideName    string         `json:"override_name,omitempty"`
	RankTier        int            `json:"rank_tier,omitempty"`
	LeaderboardRank int            `json:"leaderboard_rank,omitempty"`
	Counts          *CountResponse `json:"counts,omitempty"`
}

type PlayerFile struct {
	Players map[string]Player `json:"players"`
}

type PlayerMatch struct {
	MatchID uint64 `json:"match_id"`
}

type MatchResponse struct {
	Players []Player `json:"players"`
}

func playerPeerRequest(player Player, ch chan []Player, c *RLHTTPClient) {
	url := fmt.Sprintf("https://api.opendota.com/api/players/%v/peers?region=7&date=7&lobby_type=7", player.AccountID)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := c.Do(req)
	if err != nil {
		log.Fatalln(err)
	}
	players_resp := []Player{}
	if resp.StatusCode != 200 {
		fmt.Printf("Unexpected response %d\n", resp.StatusCode)
		ch <- players_resp
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}

	json.Unmarshal(body, &players_resp)
	ch <- players_resp
}

func playerRecentMatches(player Player, ch chan []PlayerMatch, c *RLHTTPClient) {
	url := fmt.Sprintf("https://api.opendota.com/api/players/%v/matches?region=7&date=1&lobby_type=7", player.AccountID)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := c.Do(req)
	if err != nil {
		log.Fatalln(err)
	}
	matches_resp := []PlayerMatch{}
	if resp.StatusCode != 200 {
		fmt.Printf("Unexpected response %d\n", resp.StatusCode)
		ch <- matches_resp
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}

	err = json.Unmarshal(body, &matches_resp)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(matches_resp)
	ch <- matches_resp
}

func findPlayersInMatch(matchID uint64, ch chan []Player, c *RLHTTPClient) {
	url := fmt.Sprintf("https://api.opendota.com/api/matches/%v/", matchID)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := c.Do(req)
	if err != nil {
		log.Fatalln(err)
	}

	players_resp := []Player{}
	if resp.StatusCode != 200 {
		fmt.Printf("Unexpected response %d\n", resp.StatusCode)
		ch <- players_resp
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	match_resp := MatchResponse{}

	json.Unmarshal(body, &match_resp)
	ch <- match_resp.Players
}

func findNewPlayersFromMatches(pf *PlayerFile, c *RLHTTPClient, count int) PlayerFile {
	match_ch := make(chan []PlayerMatch)
	limit := count
	for _, player := range pf.Players {
		if player.LeaderboardRank != 0 {
			go playerRecentMatches(player, match_ch, c)
			limit--
			if limit < 0 {
				break
			}
		}
	}
	unique_matches := make(map[uint64]bool)
	for i := 0; i < count; i++ {
		newmatches := <-match_ch
		for _, match := range newmatches {
			if _, ok := unique_matches[match.MatchID]; !ok {
				unique_matches[match.MatchID] = true
			}
		}
	}

	pch := make(chan []Player)

	for match_id := range unique_matches {
		go findPlayersInMatch(match_id, pch, c)
	}

	new_from_matches := PlayerFile{Players: make(map[string]Player)}
	for range unique_matches {
		newplayers := <-pch
		for _, p := range newplayers {
			accstring := fmt.Sprintf("%v", p.AccountID)
			if _, ok := pf.Players[accstring]; !ok {
				new_from_matches.Players[accstring] = p
			}
		}
	}
	return new_from_matches
}

func findNewPlayersFromPeers(players *PlayerFile, c *RLHTTPClient, count int) PlayerFile {
	pch := make(chan []Player)
	limit := count
	for _, player := range players.Players {
		go playerPeerRequest(player, pch, c)
		limit--
		if limit < 0 {
			break
		}
	}
	new_pf := PlayerFile{Players: make(map[string]Player)}
	for i := 0; i < count; i++ {
		newplayers := <-pch
		for _, p := range newplayers {
			accstring := fmt.Sprintf("%v", p.AccountID)
			if _, ok := new_pf.Players[accstring]; !ok {
				new_pf.Players[accstring] = p
			}
		}
	}
	return new_pf
}

func (pf *PlayerFile) update(new_players *PlayerFile) int {
	newcount := 0
	for accstring, p := range new_players.Players {
		if player, ok := pf.Players[accstring]; ok {
			player.PersonaName = p.PersonaName
			pf.Players[accstring] = player
		} else {
			pf.Players[accstring] = p
			newcount += 1
		}
	}
	return newcount
}

func (pf *PlayerFile) removeNonImmortals() {
	for accstring, p := range pf.Players {
		if p.RankTier != 80 {
			delete(pf.Players, accstring)
		}
	}
}

func (pf *PlayerFile) removeUnranked() {
	for accstring, p := range pf.Players {
		if p.LeaderboardRank == 0 {
			delete(pf.Players, accstring)
		}
	}
}

// func main() {
// 	players := loadPlayers("players.json")
// 	f, err := os.Open("ausnp.txt")

// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	defer f.Close()

// 	scanner := bufio.NewScanner(f)

// 	for scanner.Scan() {
// 		line := scanner.Text()
// 		if !strings.HasPrefix(line, "#") {
// 			res := strings.SplitN(line, " ", 2)
// 			if player, ok := players.Players[res[0]]; ok {
// 				player.OverrideName = res[1]
// 				players.Players[res[0]] = player
// 			}
// 		}
// 	}

// 	if err := scanner.Err(); err != nil {
// 		log.Fatal(err)
// 	}
// 	saveJSON(players, "newplayers.json")
// }

func main() {
	fmt.Printf("Starting\n")
	rl := rate.NewLimiter(rate.Every(2*time.Second), 30)
	c := NewClient(rl)
	players := loadPlayers("players.json")

	new_from_matches := findNewPlayersFromMatches(&players, c, 30)
	delete(new_from_matches.Players, "0")
	new_from_peers := findNewPlayersFromPeers(&players, c, 30)

	players.update(&new_from_peers)
	players.update(&new_from_matches)

	accountIDs := make([]uint32, len(players.Players))
	i := 0
	for _, player := range players.Players {
		accountIDs[i] = uint32(player.AccountID)
		i++
	}
	result := GetAllProfileCards(accountIDs)
	for accstring, player := range players.Players {
		player.LeaderboardRank = result[player.AccountID].LeaderboardRank
		player.RankTier = result[player.AccountID].RankTier
		players.Players[accstring] = player
	}

	players.removeNonImmortals()

	saveJSON(players, "players.json")

	players.removeUnranked()
	getPlayerCounts(&players, c)

	i = 0
	ordered_players := make([]Player, len(players.Players))
	for _, p := range players.Players {
		ordered_players[i] = p
		i++
	}

	sort.Slice(ordered_players, func(i, j int) bool {
		return ordered_players[i].LeaderboardRank < ordered_players[j].LeaderboardRank
	})
	renderHTML(ordered_players)
}

func percentage(a, b int) float64 {
	return float64(a) / float64(b)
}

var grad, _ = colorgrad.NewGradient().HtmlColors("red", "#EEEEEE", "#31e931").Domain(0.4, 0.5, 0.6).Build()

func (r RegionScore) repr() string {
	if r.Games > 0 {
		percent := percentage(r.Wins, r.Games)
		return fmt.Sprintf("%d <span style='color: %s'>(%.1f%%)</span>", r.Games, grad.At(percent).Hex(), percent*100)
	}
	return ""
}

func renderHTML(ordered_players []Player) {
	filtered := []Player{}
	for i := range ordered_players {
		if ordered_players[i].Counts != nil && ordered_players[i].LeaderboardRank != 0 && ordered_players[i].Counts.Region.AU.Games > 0 {
			filtered = append(filtered, ordered_players[i])
		}
	}

	funcMap := template.FuncMap{
		"inc": func(i int) int {
			return i + 1
		},
		"activity": func(cinfo CountResponse, region string) string {
			if region == "SEA" {
				return cinfo.Region.SEA.repr()
			}
			if region == "AU" {
				return cinfo.Region.AU.repr()
			}
			return ""
		},
	}

	templateMap := map[string]interface{}{}
	templateMap["players"] = filtered
	templateMap["time"] = time.Now().Unix()
	t := template.Must(template.New("leaderboard.gohtml").Funcs(funcMap).ParseFiles("leaderboard.gohtml"))
	f, _ := os.Create("./dist/index.html")
	err := t.Execute(f, templateMap)
	if err != nil {
		panic(err)
	}
	saveJSON(ordered_players, "ordered.json")
}

func loadPlayers(filename string) PlayerFile {
	jsonFile, _ := os.Open(filename)
	byteValue, _ := ioutil.ReadAll(jsonFile)
	pf := PlayerFile{}
	json.Unmarshal(byteValue, &pf)
	return pf
}

func saveJSON(data interface{}, filename string) {
	j, _ := json.MarshalIndent(data, "", "  ")
	_ = ioutil.WriteFile(filename, j, 0644)
}
