package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	counter    sync.Map
	zoneList   = []string{}
	quiet      = flag.Bool("quiet", false, "do not print dnstap messages on stdout")
	listenAddr = flag.String("listen", "127.0.0.1:5678", "address to listen to")
	zoneFile   = flag.String("zonefile", "/etc/unbound/local.d/dns.conf", "unbound local-data file")
)

func normalize(s string) string {
	s = strings.ToLower(s)
	if !strings.HasSuffix(s, ".") {
		s = s + "."
	}
	return "." + s
}

func loadZoneFile(file string) error {
	zoneRe := regexp.MustCompile(`local-zone:\s*\"*([^\s^\"]+)\"*\s+([\w]+)`)
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	zonemap := map[string]bool{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "local-zone:") {
			m := zoneRe.FindAllStringSubmatch(line, 1)
			if m != nil && len(m) == 1 && len(m[0]) == 3 && m[0][2] == "redirect" {
				zonemap[normalize(m[0][1])] = true
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	for k := range zonemap {
		zoneList = append(zoneList, k)
	}
	return nil
}
func processLogs() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if !*quiet {
			fmt.Println(line)
		}
		ss := strings.Split(line, " ")
		if len(ss) == 8 && ss[1] == "CQ" {
			host := strings.Trim(ss[5], "\"")
			for _, zone := range zoneList {
				if host == zone || strings.HasSuffix(host, zone) {
					px := new(uint64)
					*px = 0
					p, _ := counter.LoadOrStore(host, px)
					atomic.AddUint64(p.(*uint64), 1)
					break
				}
			}
		}
	}
}

func dumpJSON() []byte {
	ret := map[string]uint64{}
	counter.Range(func(k, v interface{}) bool {
		ret[k.(string)] = *(v.(*uint64))
		return true
	})
	retv, _ := json.MarshalIndent(ret, "", "  ")
	return retv
}

func handler(w http.ResponseWriter, r *http.Request) {
	w.Write(dumpJSON())
}

func main() {
	flag.Parse()
	counter = sync.Map{}
	loadZoneFile(*zoneFile)
	http.HandleFunc("/dump", handler)
	go func() {
		log.Fatal(http.ListenAndServe(*listenAddr, nil))
	}()
	processLogs()
}
