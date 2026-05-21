package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"
)

func status() error {
	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	if len(reg.Repos) == 0 {
		fmt.Println("aucun repo enregistré (lance `deployeur init` dans un repo)")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "REPO\tBRANCHE\tCOMMIT\tDÉPLOYÉ\tSTATUT\tDURÉE")
	for _, r := range reg.Repos {
		st := readState(r.Name)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Name, targetBranch(r.Dir), orDash(short(st.Commit)), when(st.Timestamp), statut(st), orDash(st.Duration))
	}
	return tw.Flush()
}

func statut(st state) string {
	switch {
	case st.Timestamp == "":
		return "—"
	case st.Success:
		return "OK"
	default:
		return "ÉCHEC"
	}
}

func when(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
