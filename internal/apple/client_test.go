package apple

import (
	"encoding/json"
	"testing"
)

func TestSongDTOToDomain_DropsSongsWithoutPlayParams(t *testing.T) {
	// A catalog song with no playParams cannot play in this storefront; adding
	// it succeeds and then the track is missing from the playlist.
	const body = `{"id":"1576164180","attributes":{
		"name":"I Wanna Be Your Slave","artistName":"Måneskin",
		"albumName":"BRAVO Hits, Vol. 114","durationInMillis":173000}}`
	var dead songDTO
	if err := json.Unmarshal([]byte(body), &dead); err != nil {
		t.Fatal(err)
	}
	if _, ok := dead.toDomain(); ok {
		t.Error("song without playParams must be rejected")
	}
}

func TestSongDTOToDomain_KeepsPlayableSong(t *testing.T) {
	const body = `{"id":"1556035502","attributes":{
		"name":"I WANNA BE YOUR SLAVE","artistName":"Måneskin",
		"albumName":"Teatro d'Ira - Vol. I","durationInMillis":173000,
		"isrc":"ITRSE2100014","playParams":{"id":"1556035502"}}}`
	var live songDTO
	if err := json.Unmarshal([]byte(body), &live); err != nil {
		t.Fatal(err)
	}
	got, ok := live.toDomain()
	if !ok {
		t.Fatal("playable song must be kept")
	}
	if got.ID != "1556035502" || got.ISRC != "ITRSE2100014" {
		t.Errorf("mapping lost data: %+v", got)
	}
}
