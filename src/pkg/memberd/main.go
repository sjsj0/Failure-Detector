package main

import (
	"dgm-g33/pkg/config"
	"dgm-g33/pkg/daemon"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)

	cfg, err := config.LoadFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	d, err := daemon.Run(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer d.Close()

	// Graceful shutdown on SIGINT/SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}

//package main
//
//import (
//"fmt"
//"net"
//)
//
//func main() {
//	ips, err := net.LookupIP("fa25-cs425-3301.cs.illinois.edu")
//	if err != nil {
//		panic(err)
//	}
//	for _, ip := range ips {
//		fmt.Println(ip.String())
//	}
//}

//DNS -> 130.126.2.131
//nslookup 172.22.95.38 130.126.2.131
