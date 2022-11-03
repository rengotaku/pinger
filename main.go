package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"time"

	_ "time/tzdata"

	"github.com/go-ping/ping"
	log "github.com/sirupsen/logrus"
)

var (
	logfile           *os.File
	destinationDomain *string
	interval          *int
	debugFlg          *bool
	globalIP          string
	checkinFlag       = false
)

type globIP struct {
	Query string
}

func getGlobalip() (*string, error) {
	req, err := http.Get("http://ip-api.com/json/")
	if err != nil {
		return nil, err
	}
	defer req.Body.Close()

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	var ip globIP
	json.Unmarshal(body, &ip)

	return &ip.Query, nil
}

func init() {
	os.Setenv("TZ", "Asia/Tokyo")

	logp := flag.String("log", "pinger.log", "Output path of log.")
	destinationDomain = flag.String("dest", "www.google.com", "Destination domain.")
	interval = flag.Int("interval", 3, "Ping's interval.")
	debugFlg = flag.Bool("v", false, "Verbose output somethings.")
	flag.Parse()

	// open a file
	f, err := os.OpenFile(*logp, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
	logfile = f
	if err != nil {
		log.Fatal(fmt.Sprintf("error opening file: %v", err))
	}

	if *debugFlg {
		log.SetLevel(log.DebugLevel)
	}
	log.SetOutput(logfile)
	log.SetFormatter(&log.JSONFormatter{})
}

func main() {
	defer logfile.Close()

	ip, err := getGlobalip()
	if err != nil {
		log.Fatal("Failed to retrieve global IP: ", err)
	} else {
		globalIP = *ip
	}
	// HACK: not sure about interval value
	lookupTim := time.NewTicker(time.Duration(*interval*6) * time.Second)
	go func() {
		for _ = range lookupTim.C {
			if checkinFlag {
				log.Warning("Other goroutine is working.")
				return
			}

			checkinFlag = true
			ip, err := getGlobalip()
			if err != nil {
				// Maybe do not go through here
				log.Warning("Failed to retrieve global IP: ", err)
			} else {
				globalIP = *ip
				log.Debug("Retrieved global IP: ", globalIP)
			}
			checkinFlag = false
		}
	}()

	pinger, err := ping.NewPinger(*destinationDomain)
	if err != nil {
		panic(err)
	}
	pinger.Interval = time.Duration(*interval) * time.Second

	// Listen for Ctrl-C.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for _ = range c {
			pinger.Stop()
			lookupTim.Stop()
		}
	}()

	pinger.OnRecv = func(pkt *ping.Packet) {
		s := fmt.Sprintf("%d bytes dst %s, src %s: icmp_seq=%d time=%v", pkt.Nbytes, globalIP, pkt.IPAddr, pkt.Seq, pkt.Rtt)
		log.Info(s)
	}

	pinger.OnDuplicateRecv = func(pkt *ping.Packet) {
		s := fmt.Sprintf("%d bytes dst %s, src %s: icmp_seq=%d time=%v ttl=%v (DUP!)", pkt.Nbytes, globalIP, pkt.IPAddr, pkt.Seq, pkt.Rtt, pkt.Ttl)
		log.Warn(s)
	}

	pinger.OnFinish = func(stats *ping.Statistics) {
		log.Info(fmt.Sprintf("--- %s ping statistics ---", stats.Addr))
		log.Info(fmt.Sprintf("%d packets transmitted, %d packets received, %v%% packet loss", stats.PacketsSent, stats.PacketsRecv, stats.PacketLoss))
		log.Info(fmt.Sprintf("round-trip min/avg/max/stddev = %v/%v/%v/%v", stats.MinRtt, stats.AvgRtt, stats.MaxRtt, stats.StdDevRtt))
	}

	log.Info(fmt.Sprintf("PING %s (%s):", pinger.Addr(), pinger.IPAddr()))
	err = pinger.Run()
	if err != nil {
		panic(err)
	}
}
