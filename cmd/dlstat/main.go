// Command dlstat is a developer CLI for poking at Deadlock match data.
//
//	dlstat history [-n 20]          recent matches of the account
//	dlstat match <match_id>         per-player breakdown from match metadata
//	dlstat meta <file>              same, from a local .meta file
//
// The account defaults to $DL_ACCOUNT (32-bit account id or SteamID64).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"text/tabwriter"
	"time"

	"deadlock_ideas/internal/gamedata"
	"deadlock_ideas/internal/meta"
	"deadlock_ideas/internal/pb"
	"deadlock_ideas/internal/source"
)

func main() {
	fs := flag.NewFlagSet("dlstat", flag.ExitOnError)
	account := fs.String("account", os.Getenv("DL_ACCOUNT"), "account id or SteamID64")
	n := fs.Int("n", 20, "number of matches for history")
	cacheDir := fs.String("cache", "data/cache", "replay cache directory")

	if len(os.Args) < 2 {
		usage()
	}
	cmd := os.Args[1]
	args := parseInterspersed(fs, os.Args[2:])

	ctx := context.Background()
	api := source.NewDeadlockAPI()
	cache := source.NewCache(*cacheDir, api)
	acc := parseAccount(*account)

	var err error
	switch cmd {
	case "history":
		err = history(ctx, api, acc, *n)
	case "match":
		if len(args) < 1 {
			usage()
		}
		id, perr := strconv.ParseUint(args[0], 10, 64)
		if perr != nil {
			fail(perr)
		}
		var path string
		if path, err = cache.MetadataPath(ctx, id); err == nil {
			err = showMeta(path, acc)
		}
	case "meta":
		if len(args) < 1 {
			usage()
		}
		err = showMeta(args[0], acc)
	default:
		usage()
	}
	if err != nil {
		fail(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: dlstat history|match <id>|meta <file> [-account id]")
	os.Exit(2)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func parseAccount(s string) uint32 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		fail(fmt.Errorf("bad account %q", s))
	}
	if v > 1<<32 {
		return source.SteamID64ToAccountID(v)
	}
	return uint32(v)
}

func history(ctx context.Context, api *source.DeadlockAPI, acc uint32, n int) error {
	if acc == 0 {
		return fmt.Errorf("set -account or DL_ACCOUNT")
	}
	ms, err := api.MatchHistory(ctx, acc)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MATCH\tDATE\tHERO\tRESULT\tK/D/A\tSOULS\tLEN")
	for i, m := range ms {
		if i >= n {
			break
		}
		res := "loss"
		if m.Won() {
			res = "WIN"
		}
		fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%d/%d/%d\t%d\t%s\n", m.MatchID,
			time.Unix(m.StartTime, 0).Format("01-02 15:04"), m.HeroID, res,
			m.Kills, m.Deaths, m.Assists, m.NetWorth, mmss(m.DurationS))
	}
	return w.Flush()
}

func showMeta(path string, acc uint32) error {
	mi, err := meta.ReadFile(path)
	if err != nil {
		return err
	}
	fmt.Printf("match %d  mode=%v/%v  duration=%s  winner=%v  start=%s\n",
		mi.GetMatchId(), mi.GetGameMode(), mi.GetMatchMode(), mmss(mi.GetDurationS()),
		mi.GetWinningTeam(), time.Unix(int64(mi.GetStartTime()), 0).Format(time.DateTime))
	fmt.Printf("objectives=%d  midboss=%d  paths=%v  damage_matrix=%v\n\n",
		len(mi.GetObjectives()), len(mi.GetMidBoss()), mi.GetMatchPaths() != nil, mi.GetDamageMatrix() != nil)

	players := mi.GetPlayers()
	sort.Slice(players, func(i, j int) bool {
		if players[i].GetTeam() != players[j].GetTeam() {
			return players[i].GetTeam() < players[j].GetTeam()
		}
		return players[i].GetNetWorth() > players[j].GetNetWorth()
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TEAM\tSLOT\tACCOUNT\tHERO\tLANE\tK/D/A\tSOULS\tLH\t@10m\t@20m\tSTATS\tITEMS\tDEATHS")
	var me *pb.CMsgMatchMetaDataContents_Players
	for _, p := range players {
		mark := ""
		if p.GetAccountId() == acc {
			mark, me = " <", p
		}
		fmt.Fprintf(w, "%d\t%d\t%d%s\t%s\t%d\t%d/%d/%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
			p.GetTeam(), p.GetPlayerSlot(), p.GetAccountId(), mark, heroName(p.GetHeroId()), p.GetAssignedLane(),
			p.GetKills(), p.GetDeaths(), p.GetAssists(), p.GetNetWorth(), p.GetLastHits(),
			soulsAt(p, 600), soulsAt(p, 1200), len(p.GetStats()), len(p.GetItems()), len(p.GetDeathDetails()))
	}
	w.Flush()

	if me != nil {
		showPlayer(me)
	}
	return nil
}

func showPlayer(p *pb.CMsgMatchMetaDataContents_Players) {
	fmt.Printf("\n--- you: %s ---\n", heroName(p.GetHeroId()))

	fmt.Println("\nstat snapshots (time  souls  lh  dmg  dmg_taken  death_loss):")
	for _, s := range p.GetStats() {
		fmt.Printf("  %6s %6d %4d %7d %7d %6d\n", mmss(s.GetTimeStampS()), s.GetNetWorth(),
			s.GetCreepKills(), s.GetPlayerDamage(), s.GetPlayerDamageTaken(), s.GetGoldDeathLoss())
	}

	fmt.Println("\nitems (time  name  sold):")
	for _, it := range p.GetItems() {
		sold := ""
		if it.GetSoldTimeS() > 0 {
			sold = "sold " + mmss(it.GetSoldTimeS())
		}
		fmt.Printf("  %6s  %-40s %s\n", mmss(it.GetGameTimeS()), itemName(it.GetItemId()), sold)
	}

	fmt.Println("\ndeaths (time  killer_slot  pos  respawn):")
	for _, d := range p.GetDeathDetails() {
		pos := d.GetDeathPos()
		fmt.Printf("  %6s  %2d  (%.0f, %.0f)  %ds\n", mmss(d.GetGameTimeS()), d.GetKillerPlayerSlot(),
			pos.GetX(), pos.GetY(), d.GetDeathDurationS())
	}
}

// soulsAt returns net worth at the last snapshot at or before t seconds.
func soulsAt(p *pb.CMsgMatchMetaDataContents_Players, t uint32) uint32 {
	var v uint32
	for _, s := range p.GetStats() {
		if s.GetTimeStampS() > t {
			break
		}
		v = s.GetNetWorth()
	}
	return v
}

func mmss(s uint32) string {
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// parseInterspersed lets flags appear before or after positional args.
func parseInterspersed(fs *flag.FlagSet, in []string) []string {
	var pos []string
	for {
		fs.Parse(in)
		in = fs.Args()
		if len(in) == 0 {
			return pos
		}
		pos = append(pos, in[0])
		in = in[1:]
	}
}

var gd, _ = gamedata.Embedded()

func heroName(id uint32) string {
	if h := gd.Hero(id); h != nil && h.Name != "" {
		return h.Name
	}
	return fmt.Sprintf("hero#%d", id)
}

func itemName(id uint32) string {
	a := gd.Ability(id)
	if a == nil {
		return fmt.Sprintf("#%d", id)
	}
	name := a.Name
	if name == "" {
		name = a.Key
	}
	if a.IsItem() {
		return fmt.Sprintf("%s [%s T%d]", name, a.Slot, a.Tier)
	}
	return fmt.Sprintf("%s (%s)", name, a.Type)
}
