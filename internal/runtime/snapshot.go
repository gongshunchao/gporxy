package runtime

import (
	"fmt"
	"sort"

	"gproxy/internal/config"
)

type Item struct {
	Key   string
	Entry config.Entry
}

type Snapshot struct {
	Items map[string]Item
}

type DiffResult struct {
	Add    []Item
	Remove []Item
	Keep   []Item
}

func SnapshotFromEntries(entries []config.Entry) Snapshot {
	items := make(map[string]Item, len(entries))
	for _, entry := range entries {
		key := fmt.Sprintf("%s|%s|%s", entry.Protocol, entry.Listen, entry.Target)
		items[key] = Item{
			Key:   key,
			Entry: entry,
		}
	}

	return Snapshot{Items: items}
}

func Diff(current, next Snapshot) DiffResult {
	var result DiffResult

	for key, item := range current.Items {
		if _, ok := next.Items[key]; ok {
			result.Keep = append(result.Keep, item)
			continue
		}
		result.Remove = append(result.Remove, item)
	}

	for key, item := range next.Items {
		if _, ok := current.Items[key]; ok {
			continue
		}
		result.Add = append(result.Add, item)
	}

	sort.Slice(result.Add, func(i, j int) bool { return result.Add[i].Key < result.Add[j].Key })
	sort.Slice(result.Remove, func(i, j int) bool { return result.Remove[i].Key < result.Remove[j].Key })
	sort.Slice(result.Keep, func(i, j int) bool { return result.Keep[i].Key < result.Keep[j].Key })

	return result
}
