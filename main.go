package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	_ "time/tzdata"

	"github.com/go-ping/ping"
	log "github.com/sirupsen/logrus"
	"github.com/tcnksm/go-httpstat"
)

var (
	logfile *os.File
	arg     = argument{}
)

type argument struct {
	Log               string
	DestinationDomain string
	Interval          int
	DebugFlg          bool
}

func request() (*httpstat.Result, error) {
	req, err := http.NewRequest("GET", "http://www.google.com/", nil)
	if err != nil {
		return nil, err
	}

	var result httpstat.Result
	ctx := httpstat.WithHTTPStat(req.Context(), &result)
	req = req.WithContext(ctx)

	client := http.DefaultClient
	client.Timeout = time.Duration(5) * time.Second
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(ioutil.Discard, res.Body); err != nil {
		return nil, err
	}

	return &result, nil
}

func newPinger() ping.Pinger {
	pinger, err := ping.NewPinger(arg.DestinationDomain)
	if err != nil {
		log.Error(err)
	}
	log.Debug(fmt.Sprintf("PING %s (%s):", pinger.Addr(), pinger.IPAddr()))

	pinger.Count = 5
	pinger.Interval = time.Duration(arg.Interval) * time.Second
	pinger.Timeout = time.Duration(arg.Interval*6) * time.Second

	pinger.OnRecv = func(pkt *ping.Packet) {
		s := fmt.Sprintf("%d bytes from %s: icmp_seq=%d time=%v", pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt)
		log.Debug(s)
	}

	pinger.OnDuplicateRecv = func(pkt *ping.Packet) {
		s := fmt.Sprintf("%d bytes from %s: icmp_seq=%d time=%v ttl=%v (DUP!)", pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt, pkt.Ttl)
		log.Debug(s)
	}

	pinger.OnFinish = func(stats *ping.Statistics) {
		log.Debug(fmt.Sprintf("%d packets transmitted, %d packets received, %v%% packet loss", stats.PacketsSent, stats.PacketsRecv, stats.PacketLoss))
		log.Debug(fmt.Sprintf("round-trip min/avg/max/stddev = %v/%v/%v/%v", stats.MinRtt, stats.AvgRtt, stats.MaxRtt, stats.StdDevRtt))
	}

	return *pinger
}

func flags() {
	a := flag.String("log", "pinger.log", "Output path of log.")
	b := flag.String("dest", "www.google.com", "Destination domain.")
	c := flag.Int("interval", 10, "Ping's interval.")
	d := flag.Bool("v", false, "Verbose output somethings.")
	flag.Parse()

	arg.Log = *a
	arg.DestinationDomain = *b
	arg.Interval = *c
	arg.DebugFlg = *d
}

func init() {
	os.Setenv("TZ", "Asia/Tokyo")
	flags()

	// open a file
	f, err := os.OpenFile(arg.Log, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		log.Fatal(fmt.Sprintf("error opening file: %v", err))
	}
	logfile = f

	if arg.DebugFlg {
		log.SetLevel(log.DebugLevel)
	}
	log.SetOutput(logfile)
	log.SetFormatter(&log.JSONFormatter{})
}

func main() {
	j, _ := json.Marshal(arg)
	log.Debugln(fmt.Sprintf("Arguments: %s", string(j)))

	defer logfile.Close()

	log.Debug("Trying http request...")
	_, err := request()
	if err != nil {
		log.Fatal("Error while http requesting: ", err)
	}
	log.Debug("Done http request")

	pingTim := time.NewTicker(time.Duration(arg.Interval*6) * time.Second)
	httpTim := time.NewTicker(time.Duration(arg.Interval*6) * time.Second)
	for {
		select {
		case <-pingTim.C:
			go func() {
				log.Debug("Begining ping...")
				p := newPinger()
				err = p.Run()
				if err != nil {
					log.Fatal(err)
				}
			}()
		case <-httpTim.C:
			go func() {
				log.Debug("Begining request of http...")

				statRes, err := request()
				if err != nil {
					log.Error(err)
				}
				log.Info(fmt.Sprintf("%+v", statRes))
			}()
		}
	}
}
