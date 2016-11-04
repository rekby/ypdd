package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var (
	Token    = flag.String("token", "", "OAuth token for access to domain. Default take from environment variable")
	TokenEnv = flag.String("tokenenv", "YANDEX_PDD_TOKEN", "Name of env val for get OAuth token")
	TTL      = flag.Int("ttl", 0, "TTL for add/edit record. 0 mean default yandex value (doesn't send)")
)

var (
	ExitCode = 0
)

func main() {
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
		add(domain, flag.Args()[2:]...)
	case "list":
		list(domain)
	case "del":
		if flag.NArg() < 3 {
			ErrorMessage("Need more arguments")
			break
		}
		del(domain, flag.Arg(2))
	default:
		ErrorMessage("Unknown command: %v", cmd)
	}

	os.Exit(ExitCode)
}

func Usage() {
	fmt.Printf(`%[0]v [options] domain command args

The command return code 0 for success and non 0 for error.

Commands:
	add subdomain TYPE [PRIORITY - for MX and SRV] [WEIGHT - for SRV] [PORT - for SRV] VALUE
	    TYPE - A, AAAA, CNAME, MX, NS, SOA, SRV, TXT
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

func add(domain string, args ...string) {
	if len(args) < 3 {
		ErrorMessage("Command add need more argumants")
		return
	}

	var requestArgs []string
	switch recordType := args[1]; strings.ToUpper(recordType) {
	case "MX":
		if len(args) != 4 {
			ErrorMessage("MX record need 4 command arguments: subdomain, record type, priority, content")
			return
		}
		requestArgs = []string{"domain", domain, "type", recordType, "subdomain", args[0], "priority", args[2], "content", args[3]}
	default:
		if len(args) != 3 {
			ErrorMessage("Need 3 command arguments: subdomain, record type, content")
			return
		}
		requestArgs = []string{"domain", domain, "type", recordType, "subdomain", args[0], "content", args[2]}
	}
	if *TTL != 0 {
		requestArgs = append(requestArgs, "ttl", strconv.Itoa(*TTL))
	}
	respBytes, err := pddRequest(http.MethodPost, "dns/add", requestArgs...)
	if err != nil {
		ErrorMessage(err.Error())
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

func del(domain string, id string) {
	respBytes, err := pddRequest(http.MethodPost, "dns/del", "domain", domain, "record_id", id)
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

func list(domain string) {
	respBytes, err := pddRequest(http.MethodGet, "dns/list", "domain", domain)
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

func pddRequest(method string, address string, args ...string) (body []byte, err error) {
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

	client := http.Client{}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return ioutil.ReadAll(resp.Body)
}
