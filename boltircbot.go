package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/thoj/go-ircevent"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"math"
	"unicode"
)

var ChannelUsers []string
var config = flag.String("config", "", "configuration file")

type Config struct {
	Irc struct {
		Ssl           bool     `json:"ssl"`
		SslVerifySkip bool     `json:"ssl_verify_skip"`
		Port          string   `json:"port"`
		Nickname      string   `json:"nickname"`
		Channels      []string `json:"channels"`
		Host          string   `json:"host"`
		Password      string   `json:"password"`
	} `json:"irc"`
	Github struct {
		Token string `json:"token"`
		Owner string `json:"owner"`
		Repos string `json:"repos"`
	} `json:"github"`
	Database struct {
		Karma string `json:"karma"`
	} `json:"database"`
	Logging struct {
		Location string `json:"location"`
	} `json:"logging"`
}

func (c *Config) Load(filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return err
	}

	if c.Irc.Nickname == "" {
		c.Irc.Nickname = "issuebot"
	}

	if c.Irc.Host == "" {
		return errors.New("host is required.")
	}

	if c.Github.Token == "" {
		return errors.New("token is required.")
	}

	if c.Github.Owner == "" {
		return errors.New("owner is required.")
	}

	if c.Github.Repos == "" {
		return errors.New("repos is required.")
	}

	return nil
}

func main() {
	// Load config
	flag.Parse()
	c := &Config{}
	if err := c.Load(*config); err != nil {
		log.Fatal(err)
	}

	// Logs
	logs := make(map[string]*os.File)

	ircproj := irc.IRC(c.Irc.Nickname, c.Irc.Nickname)
	ircproj.UseTLS = c.Irc.Ssl
	if c.Irc.SslVerifySkip {
		ircproj.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}
	ircproj.Password = c.Irc.Password
	ircproj.QuitMessage = "Drop bear spotted… I'm out of here!"
	
	err := ircproj.Connect(net.JoinHostPort(c.Irc.Host, c.Irc.Port))
	if err != nil {
		log.Fatal(err)
	}

	// Create a channel to receive signal notifications 
	sigs := make(chan os.Signal, 1)
	// Register the channel to receive notifications SIGINT & SIGTERM signals.
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		ircproj.Quit()
		fmt.Println()
		fmt.Println(fmt.Sprintf("Received %v signal", sig))
	}()

	ircproj.AddCallback("001", func(event *irc.Event) {
		for _, channel := range c.Irc.Channels {
			ircproj.Join(channel)
			log.Println(fmt.Sprintf("Joined %v", channel))

			// Start the logger for this channel
			logs[channel] = StartLogger(c, channel)

			// Set the log to close on exit
			//defer logs[channel].Close()
		}
	})

	// Logging
	ircproj.AddCallback("PRIVMSG", func(event *irc.Event) {
		channel := event.Arguments[0]
		WriteLog(c, logs[channel], event.Nick, event.Message())
	})

	r := regexp.MustCompile(`#(-?\d+)`)
	ircproj.AddCallback("PRIVMSG", func(event *irc.Event) {
		matches := r.FindAllStringSubmatch(event.Message(), -1)
		for _, match := range matches {
			// Don't start a bot war
			if event.Nick == "[BoltGitHubBot]" {
				continue
			}
			if len(match) < 2 {
				continue
			}
			matchNormalized, err := strconv.ParseFloat(match[1], 64)
			matchNegative := math.Signbit(matchNormalized)
			gitMatch := strconv.FormatFloat(math.Abs(matchNormalized), 'f', -1, 64)
			u, err := url.Parse(fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s", c.Github.Owner, c.Github.Repos, gitMatch))
			if err != nil {
				log.Println(err)
				continue
			}
			q := u.Query()
			q.Add("access_token", c.Github.Token)
			u.RawQuery = q.Encode()
			resp, err := http.Get(u.String())
			if err != nil {
				log.Println(err)
				continue
			}
			if !(200 <= resp.StatusCode && resp.StatusCode <= 299) {
				log.Println(resp.Status)
				continue
			}
			defer resp.Body.Close()
			m := make(map[string]interface{})
			if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
				log.Println(err)
				continue
			}
			if matchNegative {
				if matchNormalized == -1 {
					ircproj.Noticef(event.Arguments[0], "#%v Add locale ´Rövarspråket´ as default, since it is the new Lingua Franca of the internet. https://github.com/bolt/bolt/issues/1", m["number"].(float64))
				} else if matchNormalized == -1555 {
					ircproj.Noticef(event.Arguments[0], "Do not tempt the gods, %v.", event.Nick)
				} else if math.Mod(m["number"].(float64), 1555) == 0 {
					ircproj.Noticef(event.Arguments[0], "#%v They don't think it be like it is but it do. https://github.com/bolt/bolt/issues/%v", m["number"].(float64), m["number"].(float64))
				} else {
					ircproj.Noticef(event.Arguments[0], "#%v %v %v", m["number"].(float64), swedishEncode(m["title"].(string)), m["html_url"].(string))
				}
			} else if m["number"].(float64) == 1 {
				// I am a bot, I can have my own rule #1
				ircproj.Noticef(event.Arguments[0], "#1 Port Bolt to Go to keep %v happy https://github.com/bolt/bolt/issues/1", c.Irc.Nickname)
				time.Sleep(5 * time.Second)
				ircproj.Action(event.Arguments[0], "is written in Go, and therefore isn't allowed to like PHP")
			} else if m["number"].(float64) == 1555 {
				// Props to Adrian Guenter
				ircproj.Actionf(event.Arguments[0], "warns %v that #1555 nearly caused the end of the known universe and should never be mentioned again", event.Nick)
			} else {
				assignee := ""
				if m["assignee"] != nil {
					assignee = fmt.Sprintf(" — assigned to %v", m["assignee"].(map[string]interface{})["login"].(string))
				}
				ircproj.Noticef(event.Arguments[0], "#%v [%s] %s %s %s", m["number"].(float64), m["state"].(string), m["title"].(string), m["html_url"].(string), assignee)

				if math.Mod(m["number"].(float64), 1555) == 0 {
					time.Sleep(2 * time.Second)
					ircproj.Action(event.Arguments[0], "looks at that number suspiciously…")
				}
			}
		}
	})

	// Help
	//AddHelp(ircproj)

	// Get a list of users and remove the "@" sign for chanops
	ircproj.AddCallback("353", func(event *irc.Event) {
		s := strings.Replace(event.Arguments[3], "@", "", -1)
		ChannelUsers = strings.Fields(s)
	})

	// Just for Bopp, for now
	//ircproj.AddCallback("JOIN", func(event *irc.Event) {
	//	if event.Nick == "Bopp" {
	//		time.Sleep(5 * time.Second)
	//		ircproj.Privmsgf(event.Arguments[0], RandomMessage(), event.Nick)
	//	}
	//})

	// Asimov's Laws - Three Laws of Robotics
	AddPrivmsgRules(ircproj)
	// Documentation for bolt
	AddPrivmsgDocs(ircproj)

	AddActionf(ircproj, `#(kitten|cat)`, "starts to meow at %v… *purr* *purr*")
	AddActionf(ircproj, `#dog`, "rolls over, and wants its tummy scratched by %v")
	AddActionf(ircproj, `#champagne`, "opens a nice chilled bottle of Moët & Chandon for %v")
	AddActionf(ircproj, `#beer`, "$this->app['bartender']->setDrink('beer')->setTab('%v')->serveAll();")
	AddActionf(ircproj, `#coffee`, "turns on the espresso machine for %v")
	AddActionf(ircproj, `#hotchocolate`, "believes in miracles, %v, you sexy thing!")
	AddActionf(ircproj, `#tea`, "has boiled some water, and begins to brew %v a nice cup of tea.")
	AddActionf(ircproj, `#wine`, "opens a bottle of Château Lafite at %v's request!")
	AddActionf(ircproj, `#whisky`, "pours a nip of Glenavon Special for %v.")
	AddActionf(ircproj, `#whiskey`, "takes a swig of Jameson, hands the bottle to %v, and sings - \"Whack fol de daddy-o, There's whiskey in the jar.\"")
	AddActionf(ircproj, `#shiraz`, "wonders if %v has ever had a Heathcote Estate Shiraz?")
	AddActionf(ircproj, `#rum`, "grabs a bottle of rum, passes it to %v and starts singing pirate songs")
	AddActionf(ircproj, `#water`, "pours water over %v…  That is what they wanted, right?")
	AddActionf(ircproj, `#(PR|pr|Pr|pR)`, "gets the idea that Bopp should take care of %v's pull requests or kittens may cry…")
	AddActionf(ircproj, `#vodka`, "opens a bottle of Billionaire Vodka for %v.  It's good to be the king after all!")
	//AddActionf(ircproj, `bolt`, "calls capital_B_dangit() on %v's behalf")
	AddActionf(ircproj, `#koala`, "passes some eucalyptus leaves to %v.")
	AddActionf(ircproj, `#ninja`, "visits http://%v.is-a-sneaky.ninja/")
	AddActionf(ircproj, `#upstream`, "Maybe somebody screwed up somewhere... Perhaps %v knows what happened?")

	AddAction(ircproj, `#popcorn`, "yells: POPCORN! GET YOUR POPCORN!")
	AddAction(ircproj, `#pastebin`, "asks that http://pastebin.com/ be used for more than one-line messages. It makes life easier.")
	AddAction(ircproj, `#(pony|mylittlepony)`, "says \"ZA̡͊͠͝LGΌ ISͮ̂҉̯͈͕̹̘̱ TO͇̹̺ͅƝ̴ȳ̳ TH̘Ë͖́̉ ͠P̯͍̭O̚​N̐Y̡ H̸̡̪̯ͨ͊̽̅̾̎Ȩ̬̩̾͛ͪ̈́̀́͘ ̶̧̨̱̹̭̯ͧ̾ͬC̷̙̲̝͖ͭ̏ͥͮ͟Oͮ͏̮̪̝͍M̲̖͊̒ͪͩͬ̚̚͜Ȇ̴̟̟͙̞ͩ͌͝S̨̥̫͎̭ͯ̿̔̀ͅ\"")
	AddAction(ircproj, `#tequila`, "drinks one Tequila, two Tequilas, three Tequilas… floor!")
	AddActionSilentWorks(ircproj, `(WP|wp|Wordpress|WordPress|wordpress)`, "notes that if code was poetry, WordPress would have been written in Go…  It's more like \"code is pooetry if you ask this bot\"")
	AddAction(ircproj, `#nicotine`, "coughs and opens the windows…")
	AddAction(ircproj, `OCD`, "s/OCD/CDO/ …must be in alphabetical order…")
	AddAction(ircproj, `#git`, "says you have three choices: 1. man git, 2. nicely ask gawainlynch, or 3. do it the xkcd way: https://xkcd.com/1597/")

	AddAction(ircproj, `#(BPFL|bpfl)`, "exclaims loudly: 'All bow for our Benevolent Princess for Life, the Monarch of Australia, strangler of drop bears and catcher of koalas: gawainlynch!'")
	// Someone might want to change this to something LOTR themed to fit better with Bopps interests
	AddAction(ircproj, `#(BDFL|bdfl|BoltBorn|Boltborn|boltborn)`, "starts to sing: 'Boltborn, Boltborn, by his honor is sworn, to keep featurebloat forever at bay! And the fiercest foes rout when they hear our BDFL's shout, Boltborn, for your blessing we pray!'")

	//Todo: allow "koala" to be lowercase without conflicting with the #koala directive (problem with golang not allowing regex lookahead)
	AddAction(ircproj, `#(KoalaBugs|Koalabugs)`, "thinks he saw something small and furry scurry away from github. Somebody better check for #KoalaBugs...")
	AddAction(ircproj, `#(http418|http 418)`, "418 I'm a teapot")
	AddActionf(ircproj, `#(friday|Friday)`, "assumes that %v will spend all weekend fixing bugs in Bolt, right?")
	AddActionf(ircproj, `#soup`, "pours %v a nice warm bowl of soup")

	AddActionKarma(c, ircproj)

	AddActionInsult(c, ircproj)

	// AddTobias(ircproj)

	ircproj.Loop()
}


const Consonants = "BCDGFHJKLMNPQRSTVWXZbcdfghjklmnpqrstvwxz"

func swedishEncode(inputString string) string {
	var encodedString []rune

	for _, letter := range inputString {
		encodedString = append(encodedString, letter)

		if strings.ContainsRune(Consonants, letter) {
			encodedString = append(encodedString, 'o')
			encodedString = append(encodedString, unicode.ToLower(letter))
		}
	}

	return string(encodedString)
}

