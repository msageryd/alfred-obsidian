package main

import (
	"os"

	"github.com/drgrib/alfred"

	"github.com/msageryd/alfred-obsidian/core"
	"github.com/msageryd/alfred-obsidian/db"
)

func main() {
	query := core.ParseQuery(os.Args[1])

	litedb, err := db.NewBearDB()
	if err != nil {
		panic(err)
	}

	autocompleted, err := core.AutocompleteTags(litedb, query)
	if err != nil {
		panic(err)
	}

	if !autocompleted {
		item, err := core.GetCreateItem(query)
		if err != nil {
			panic(err)
		}
		alfred.Add(*item)
	}

	alfred.Run()
}
