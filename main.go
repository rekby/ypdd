package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/miekg/dns"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	Token    = flag.String("token", "", "OAuth token for access to domain. Default take from environment variable")
	TokenEnv = flag.String("tokenenv", "YANDEX_PDD_TOKEN", "Name of env val for get OAuth token")
	TTL      = flag.Int("ttl", 0, "TTL for add/edit record. 0 mean default yandex value (doesn't send)")
	Timeout  = flag.Int("timeout", 60, "Max time execution time, include result waiting (in seconds). Zero mean infinite.")
	Sync     = flag.Bool("sync", false, "Wait while record will really add to dns servers.")
	CheckInterval = flag.Int("check-interval", 1, "Pause between check records in sync mode.")
	CheckPerServer = flag.Int("request-times", 10, "How many times check every server for every check in sync mode.")
	DNSNetwork = flag.String("dns-network", "tcp", "Check dns records by tcp or udp in sync mode.")
)

var (
	ExitCode = 0
)

func main() {
	ctx := context.Background()
	if *Timeout > 0 {
		var cancelFunc context.CancelFunc
		ctx, cancelFunc = context.WithTimeout(ctx, time.Duration(*Timeout)*time.Second)
		defer cancelFunc()
	}

	flag.Usage = Usage
	flag.Parse()

	if flag.NArg() < 2 {
		Usage()
		return
	}

	if *Token == "" {
		*Token = os.Getenv(*TokenEnv)
	}
	domain := flag.Arg(0)
	switch cmd := flag.Arg(1); cmd {
	case "add":
		added := add(ctx, domain, flag.Args()[2:]...)
		if added {
			if *Sync {
				subdomain := flag.Arg(2)
				record := subdomain + "." + domain
				if !strings.HasSuffix(record, ".") {
					record += "."
				}
				recordType := flag.Arg(3)
				value := flag.Arg(flag.NArg() - 1)
				deadline, hasDeadline := ctx.Deadline()
				for {
					log.Println("Check record")
					if checkRecord(ctx, record, recordType, value) {
						break
					} else {
						if ctx.Err() != nil {
							ErrorMessage("Timeout")
							break
						}
						if !hasDeadline || time.Now().Add(time.Second * *CheckInterval).Before(deadline) {
							time.Sleep(time.Second)
						}
					}
				}
			}
			if ExitCode == 0 {
				fmt.Println("OK")
			}
		}
	case "list":
		list(ctx, domain)
	case "del":
		if flag.NArg() < 3 {
			ErrorMessage("Need more arguments")
			break
		}
		del(ctx, domain, flag.Arg(2))
	default:
		ErrorMessage("Unknown command: %v", cmd)
	}

	if ctx.Err() != nil {
		ErrorMessage("Timeout")
	}
	os.Exit(ExitCode)
}

func checkRecord(ctx context.Context, record, recordTypeString, value string) bool {
	// Yandex dns have multiply servers per every ip.
	// It mean - once good answer doesn't gurantee for next answer will good too.
	// Becouse sync they servers take some time.
	for i := 0; i < *CheckPerServer; i++{
		if !checkRecordOnce(ctx, record, recordTypeString, value) {
			return false
		}
	}
	return true
}

func checkRecordOnce(ctx context.Context, record, recordTypeString, value string) bool {
	recordType := dns.StringToType[strings.ToUpper(recordTypeString)]
	if recordType == dns.TypeNone {
		log.Println("Unknow record type:", recordTypeString)
		return false
	}

	ch := make(chan bool, 2)
	go func() { ch <- checkRecordOnServer(ctx, "dns1.yandex.ru:53", recordType, record, value) }()
	go func() { ch <- checkRecordOnServer(ctx, "dns2.yandex.ru:53", recordType, record, value) }()
	for i := 0; i < 2; i++ {
		if <-ch == false {
			return false
		}
	}
	return true
}

func checkRecordOnServer(ctx context.Context, server string, recordType uint16, record, value string) bool {
	client := &dns.Client{}
	client.Net = *DNSNetwork
	client.DialTimeout = time.Second
	client.ReadTimeout = time.Second
	client.WriteTimeout = time.Second

	msg := &dns.Msg{}
	msg.Id = dns.Id()
	msg.SetQuestion(record, recordType)

	answer, _, err := client.Exchange(msg, server)
	if err != nil {
		log.Printf("Can't read answer from dns server '%v': %v\n", server, err)
		return false
	}
	if answer.Id != msg.Id {
		log.Println("Bad answer id")
		return false
	}

	for _, r := range answer.Answer {
		if r.Header().Rrtype != recordType {
			continue
		}
		var res bool
		switch recordType {
		case dns.TypeA:
			res = r.(*dns.A).A.Equal(net.ParseIP(value))
		case dns.TypeAAAA:
			res = r.(*dns.AAAA).AAAA.Equal(net.ParseIP(value))
		case dns.TypeCNAME:
			res = r.(*dns.CNAME).Target == value
		case dns.TypeMX:
			res = r.(*dns.MX).Mx == value
		case dns.TypeNS:
			res = r.(*dns.NS).Ns == value
		case dns.TypeSRV:
			res = r.(*dns.SRV).Target == value
		case dns.TypeTXT:
			for _, txt := range r.(*dns.TXT).Txt {
				if txt == value {
					res = true
					break
				}
			}
		default:
			log.Printf("Can't cpecific check for record type '%v', check it by contains substring '%v' in record: %v\n",
				dns.TypeToString[recordType], value, r.String())
			res = strings.Contains(r.String(), value)
		}
		if res {
			return true
		}
	}
	return false
}

func Usage() {
	fmt.Printf(`%[0]v [options] domain command args

The command return code 0 for success and non 0 for error.

Commands:
	add subdomain TYPE [PRIORITY - for MX and SRV] [WEIGHT - for SRV] [PORT - for SRV] VALUE
	    TYPE - A, AAAA, CNAME, MX, NS, SRV, TXT
            output:
                OK - for succesfull add record.
                ERROR: error message - for error.
        del ID - remove record with ID
            output:
                OK - for succesfull delete record
                ERROR: error message - for error

        list - list all subdomains in format:
             output  (one record per line):
                   ID SUBDOMAIN TYPE TTL [PRIORITY - for MX and SRV] CONTENT

Example:
%[0]v --ttl 60 test.ru add sub A 127.0.0.1 # Add A record 127.0.0.1 for domain sub.test.ru with TTL 60 seconds.

Options:
`, os.Args[0])
	flag.PrintDefaults()
}

func ErrorMessage(mess ...interface{}) {
	ExitCode = 1

	switch len(mess) {
	case 0:
		fmt.Println("ERROR")
	case 1:
		fmt.Println("ERROR:", mess[0])
	default:
		fmt.Printf("ERROR: "+fmt.Sprint(mess[0])+"\n", mess[1:]...)
	}
}

func add(ctx context.Context, domain string, args ...string) bool {
	if len(args) < 3 {
		ErrorMessage("Command add need more argumants")
		return false
	}

	var requestArgs []string
	switch recordType := args[1]; strings.ToUpper(recordType) {
	case "MX":
		if len(args) != 4 {
			ErrorMessage("MX record need 4 command arguments: subdomain, record type, priority, content")
			return false
		}
		requestArgs = []string{"domain", domain, "type", recordType, "subdomain", args[0], "priority", args[2], "content", args[3]}
	case "SRV":
		if len(args) != 6 {
			ErrorMessage("MX record need 4 command arguments: subdomain, record type, priority, content")
			return false
		}
		requestArgs = []string{"domain", domain, "type", recordType, "subdomain", args[0], "priority", args[2],
			"weight", args[3], "port", args[4], "target", args[5]}
	default:
		if len(args) != 3 {
			ErrorMessage("Need 3 command arguments: subdomain, record type, content")
			return false
		}
		requestArgs = []string{"domain", domain, "type", recordType, "subdomain", args[0], "content", args[2]}
	}
	if *TTL != 0 {
		requestArgs = append(requestArgs, "ttl", strconv.Itoa(*TTL))
	}
	respBytes, err := pddRequest(ctx, http.MethodPost, "dns/add", requestArgs...)
	if err != nil {
		ErrorMessage(err.Error())
		return false
	}

	var resp struct {
		Success string `json:"success"`
		Error   string `json:"error"`
	}
	err = json.Unmarshal(respBytes, &resp)
	if err != nil {
		ErrorMessage("Parse PDD answer: %v", err)
		return false
	}
	if resp.Success != "ok" {
		ErrorMessage(resp.Error)
		return false
	}
	return true
}

func del(ctx context.Context, domain string, id string) {
	respBytes, err := pddRequest(ctx, http.MethodPost, "dns/del", "domain", domain, "record_id", id)
	if err != nil {
		ErrorMessage(err)
		return
	}
	var resp struct {
		Success string `json:"success"`
		Error   string `json:"error"`
	}
	err = json.Unmarshal(respBytes, &resp)
	if err != nil {
		ErrorMessage("Parse PDD answer: %v", err)
		return
	}
	if resp.Success != "ok" {
		ErrorMessage(resp.Error)
		return
	}
	fmt.Println("OK")
}

func list(ctx context.Context, domain string) {
	respBytes, err := pddRequest(ctx, http.MethodGet, "dns/list", "domain", domain)
	if err != nil {
		ErrorMessage(err)
		return
	}
	var resp struct {
		Success string `json:"success"`
		Error   string `json:"error"`

		Records []struct {
			ID        int         `json:"record_id"`
			Type      string      `json:"type"`
			TTL       int         `json:"ttl"`
			Subdomain string      `json:"subdomain"`
			Priority  interface{} `json:"priority"`
			Content   string      `json:"content"`
		} `json:"records"`
	}
	err = json.Unmarshal(respBytes, &resp)
	if err != nil {
		ErrorMessage("Parse PDD answer: %s", err)
		return
	}
	if resp.Success != "ok" {
		ErrorMessage(resp.Error)
		return
	}
	for _, r := range resp.Records {
		switch r.Type {
		case "MX", "SRV":
			fmt.Printf("%v %v %v %v %v %v\n", r.ID, r.Subdomain, r.Type, r.TTL, r.Priority, r.Content)
		default:
			fmt.Printf("%v %v %v %v %v\n", r.ID, r.Subdomain, r.Type, r.TTL, r.Content)
		}
	}
}

func pddRequest(ctx context.Context, method string, address string, args ...string) (body []byte, err error) {
	if len(args)%2 != 0 {
		panic("Need pairs arguments: name, value, name, value,...")
	}

	params := url.Values(map[string][]string{})
	for i := 0; i < len(args); i += 2 {
		params.Add(args[i], args[i+1])
	}
	reqUrl := "https://pddimp.yandex.ru/api2/admin/" + address
	req, err := http.NewRequest(method, reqUrl, strings.NewReader(params.Encode()))
	if err != nil {
		panic(err)
	}
	req.Header.Add("PddToken", *Token)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(ctx)

	client := http.Client{}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return ioutil.ReadAll(resp.Body)
}
