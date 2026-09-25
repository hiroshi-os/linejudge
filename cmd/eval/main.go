package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/hiroshi-os/linejudge/internal/evalx"
	"github.com/hiroshi-os/linejudge/internal/fixtures"
	"github.com/hiroshi-os/linejudge/internal/types"
)

func main() {
	out := flag.String("out", "data/eval-report.json", "write JSON report")
	approve := flag.Bool("approve-llm", true, "run the llm-assist stage (mock unless OPENAI_API_KEY is set)")
	flag.Parse()

	cfg := types.DefaultConfig()
	rep, err := evalx.Run(context.Background(), evalx.Options{
		FixturesDir: fixtures.Dir(),
		Config:      cfg,
		ApproveLLM:  *approve,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	evalx.Print(rep)
	if err := os.MkdirAll("data", 0o755); err == nil {
		if err := evalx.WriteFile(*out, rep); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("wrote", *out)
	}
}
