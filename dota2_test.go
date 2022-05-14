package main

import (
	"fmt"
	"testing"
)

func TestGetProfileCards(t *testing.T) {
	players := []uint32{46009751}
	resp := GetAllProfileCards(players)
	fmt.Print(resp)
}
