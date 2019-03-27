package main

import "flag"

var (
	upaddr      = flag.String("upaddr", "", "up addrees")
	downaddr    = flag.String("downaddr", "", "down address")
	concurrency = flag.Int("concurrency", 1, "concurrency connection")
)

func main() {
	flag.Parse()
	proxy := NewProxy(&Config{
		UpAddr:      *upaddr,
		DownAddr:    *downaddr,
		Concurrency: *concurrency,
	})
	proxy.Start()
}
