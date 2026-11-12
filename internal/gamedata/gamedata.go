// Package gamedata provides static game data (heroes, items, abilities)
// extracted from the local Deadlock install by tools/gamedata and embedded
// as a JSON snapshot, so the server doesn't need the game files at runtime.
package gamedata

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Hero is a playable (or in-development) hero.
type Hero struct {
	ID       uint32 `json:"id"`
	Key      string `json:"key"` // e.g. "hero_haze"
	Name     string `json:"name"`
	NameRU   string `json:"name_ru,omitempty"`
	Type     string `json:"type,omitempty"`  // e.g. "assassin"
	State    string `json:"state,omitempty"` // e.g. "release"
	Disabled bool   `json:"disabled,omitempty"`
	InDev    bool   `json:"in_dev,omitempty"`
	Image    string `json:"image,omitempty"` // game-internal path of the small icon
}

// Ability is anything referenced by item_id in match metadata: shop items,
// hero abilities and the like. ID is MurmurHash2(lower(key), 0x31415926).
type Ability struct {
	ID     uint32 `json:"id"`
	Key    string `json:"key"` // e.g. "upgrade_rapid_rounds"
	Name   string `json:"name,omitempty"`
	NameRU string `json:"name_ru,omitempty"`
	Type   string `json:"type"`           // item | signature | weapon | innate | ...
	Slot   string `json:"slot,omitempty"` // weapon | vitality | spirit (items only)
	Tier   int    `json:"tier,omitempty"` // 1..5 (items only)
	Image  string `json:"image,omitempty"`
}

// IsItem reports whether this is a purchasable shop item.
func (a *Ability) IsItem() bool { return a.Type == "item" }

// Data is the full snapshot.
type Data struct {
	GameVersion string     `json:"game_version,omitempty"`
	Heroes      []*Hero    `json:"heroes"`
	Abilities   []*Ability `json:"abilities"`

	heroByID    map[uint32]*Hero
	abilityByID map[uint32]*Ability
}

func (d *Data) index() {
	d.heroByID = make(map[uint32]*Hero, len(d.Heroes))
	for _, h := range d.Heroes {
		d.heroByID[h.ID] = h
	}
	d.abilityByID = make(map[uint32]*Ability, len(d.Abilities))
	for _, a := range d.Abilities {
		d.abilityByID[a.ID] = a
	}
}

// Hero returns a hero by id, or nil.
func (d *Data) Hero(id uint32) *Hero { return d.heroByID[id] }

// Ability returns an ability/item by its hashed id, or nil.
func (d *Data) Ability(id uint32) *Ability { return d.abilityByID[id] }

// Parse decodes a JSON snapshot.
func Parse(b []byte) (*Data, error) {
	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("gamedata: %w", err)
	}
	d.index()
	return &d, nil
}

//go:embed snapshot/gamedata.json
var snapshot []byte

var (
	once     sync.Once
	embedded *Data
	embedErr error
)

// Embedded returns the snapshot compiled into the binary.
func Embedded() (*Data, error) {
	once.Do(func() { embedded, embedErr = Parse(snapshot) })
	return embedded, embedErr
}

// HashKey computes the id the game uses for an ability/item name.
func HashKey(key string) uint32 {
	return murmur2([]byte(strings.ToLower(key)), 0x31415926)
}

// murmur2 is MurmurHash2 (32-bit), as used by Source 2 string tokens.
func murmur2(data []byte, seed uint32) uint32 {
	const m = 0x5bd1e995
	h := seed ^ uint32(len(data))
	for len(data) >= 4 {
		k := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
		k *= m
		k ^= k >> 24
		k *= m
		h *= m
		h ^= k
		data = data[4:]
	}
	switch len(data) {
	case 3:
		h ^= uint32(data[2]) << 16
		fallthrough
	case 2:
		h ^= uint32(data[1]) << 8
		fallthrough
	case 1:
		h ^= uint32(data[0])
		h *= m
	}
	h ^= h >> 13
	h *= m
	h ^= h >> 15
	return h
}
