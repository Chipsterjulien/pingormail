package main

import (
	"crypto/tls"
	"fmt"
	"github.com/jordan-wright/email"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	// confPath := "cfg/"
	// confFilename := "pingormail"
	// logFilename := "error.log"

	confPath := "/etc/pingormail/"
	confFilename := "pingormail"
	logFilename := "/var/log/pingormail/error.log"

	fd := initLogging(&logFilename)
	defer fd.Close()

	loadConfig(&confPath, &confFilename)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-t":
			// Send email test
			str := "fictive.ip.org"
			retry := viper.GetInt("default.retry")
			sendMail(&str, &retry)
		default:
			// Fonction help
			argSplitted := strings.Split(os.Args[0], "/")
			nameApp := argSplitted[len(argSplitted)-1]
			fmt.Printf("Usage: %s <option>\n<option>:\n  -t\tsend a false email in order to test sends\n", nameApp)
		}
	} else {
		startApp()
	}
}

func isGood(out string) bool {
	outSplitted := strings.Split(out, "\n")

	for _, line := range outSplitted {
		if strings.Contains(line, " 0% packet loss") {
			return true
		}
	}

	return false
}

func pingAddr(ipList *[]string, maxRetry *int) {
	log := logging.MustGetLogger("log")

	log.Debug("Method: ping")

	for _, ip := range *ipList {
		log.Debugf("Ping: %s", ip)
		loop := true
		counter := 0

		for loop {
			log.Debugf("Test %d of address %s", counter+1, ip)
			out, _ := exec.Command("/bin/sh", "-c", fmt.Sprintf("ping -c 1 %s", ip)).Output()
			if isGood(string(out)) {
				log.Debug("Ok")
				loop = false
			} else {
				counter++
				if counter >= *maxRetry {
					loop = false
					log.Debug("Fail")
					sendMail(&ip, maxRetry)
				} else {
					log.Debug("Fail")
				}
			}
		}
	}
}

func statusAddr(ipList *[]string, maxRetry *int) {
	log := logging.MustGetLogger("log")

	log.Debug("Method: status")

	for _, ip := range *ipList {
		log.Debugf("Ping: %s", ip)
		loop := true
		counter := 0

		for loop {
			log.Debugf("Test %d of address %s", counter+1, ip)
			if statusFunc(&ip) {
				log.Debug("Ok")
				loop = false
			} else {
				counter++
				if counter >= *maxRetry {
					loop = false
					log.Debug("Fail")
					sendMail(&ip, maxRetry)
				} else {
					log.Debug("Fail")
				}
			}
		}
	}
}

func statusFunc(ip *string) bool {
	log := logging.MustGetLogger("log")

	timeout := viper.GetInt("default.timeout")

	log.Debug("Method: status")
	log.Debugf("Timeout: %ss", timeout)

	var resp *http.Response
	var client *http.Client

	client = &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	resp, err := client.Get(*ip)
	if err != nil {
		// S'il y a une erreur, on tente de désactiver la protection ssl
		log.Warningf("There is an error to connect at %s: %s. Trying without ssl", *ip, err)

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			Dial: (&net.Dialer{
				Timeout:   time.Duration(timeout) * time.Second,
				KeepAlive: time.Duration(timeout) * time.Second,
			}).Dial,
			TLSHandshakeTimeout:   time.Duration(timeout) * time.Second,
			ResponseHeaderTimeout: time.Duration(timeout) * time.Second,
			ExpectContinueTimeout: time.Duration(timeout) * time.Second,
		}
		client := &http.Client{
			Transport: tr,
			Timeout:   time.Duration(timeout) * time.Second,
		}

		resp, err = client.Get(*ip)
		if err != nil {
			log.Criticalf("Unable to get %s: %s", *ip, err)

			return false
		}
		if resp.StatusCode == 200 {
			log.Debugf("Without ssl, I'm connected at %s and status code is 200", *ip)

			return true
		} else {
			log.Warningf("Without ssl, I'm connected at %s but status is not 200 (%d)", *ip, resp.StatusCode)

			return true
		}
	}
	if resp.StatusCode == 200 {
		log.Debugf("I'm connected at %s and status code is 200", *ip)

		return true
	} else {
		log.Warningf("I'm connected at %s but status is not 200 (%d)", *ip, resp.StatusCode)

		return true
	}
}

func sendMail(ip *string, maxRetry *int) {
	log := logging.MustGetLogger("log")

	host := viper.GetString("email.smtp")
	login := viper.GetString("email.login")
	password := viper.GetString("email.password")
	hostport := host + ":" + viper.GetString("email.port")
	from := viper.GetString("email.from")
	to := viper.GetStringSlice("email.sendTo")
	who := viper.GetString("location")

	// // TLS config
	// tlsconfig := &tls.Config{
	// 	InsecureSkipVerify: true,
	// 	ServerName:         host,
	// }

	e := email.NewEmail()
	e.From = from
	e.To = to
	e.Subject = "Machine not found"
	e.Text = []byte(fmt.Sprintf("I'm \"%s\". After %d tests, I can't ping \"%s\" !", who, *maxRetry, *ip))
	// if err := e.SendWithTLS(hostport, smtp.PlainAuth("", login, password, host), tlsconfig); err != nil {
	if err := e.Send(hostport, smtp.PlainAuth("", login, password, host)); err != nil {
		log.Warningf("Unable to send an email to \"%s\": %v", err)
	} else {
		log.Debugf("Email was sent to \"%s\"", *ip)
	}
}

func startApp() {
	log := logging.MustGetLogger("log")

	pingIpList := viper.GetStringSlice("ping.urls")
	statusIpList := viper.GetStringSlice("status.urls")
	maxRetry := viper.GetInt("default.retry")
	waitingTime := viper.GetInt("default.waiting")
	daemonMode := viper.GetBool("default.daemon")

	log.Debugf("Ip list: %s", pingIpList)
	log.Debugf("Number of tests before sending an email: %d", maxRetry)
	log.Debugf("Waiting time between two loops: %d", waitingTime)
	log.Debugf("Daemon mode: %v", daemonMode)

	if waitingTime == 0 {
		log.Warning("Waiting time must be over 0 !")
		return
	}

	for {
		pingAddr(&pingIpList, &maxRetry)
		statusAddr(&statusIpList, &maxRetry)
		if daemonMode {
			log.Debugf("Time to sleep: %d second(s)", waitingTime)
			time.Sleep(time.Duration(waitingTime) * time.Second)
		} else {
			break
		}
	}
}
