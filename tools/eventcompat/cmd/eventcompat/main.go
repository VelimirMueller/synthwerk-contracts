// Command eventcompat fails when an event schema change breaks a consumer.
//
//	go run ./tools/eventcompat/cmd/eventcompat -base <old events dir> -head events
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/VelimirMueller/synthwerk-contracts/tools/eventcompat"
)

func main() {
	base := flag.String("base", "", "events dir of the published version (for example the base branch)")
	head := flag.String("head", "events", "events dir of the new version")
	flag.Parse()
	if *base == "" {
		fmt.Fprintln(os.Stderr, "eventcompat: -base is required")
		os.Exit(2)
	}

	findings, err := eventcompat.CompareDirs(*base, *head)
	if err != nil {
		fmt.Fprintln(os.Stderr, "eventcompat:", err)
		os.Exit(2)
	}
	breaking := 0
	for _, f := range findings {
		fmt.Println(f)
		if f.Severity == eventcompat.Breaking {
			breaking++
		}
	}
	fmt.Printf("eventcompat: %d breaking, %d warnings\n", breaking, len(findings)-breaking)
	if breaking > 0 {
		fmt.Println("eventcompat: publish the change as a new type .v<N+1> (docs/naming.md)")
		os.Exit(1)
	}
}
