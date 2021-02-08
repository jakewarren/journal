package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/gookit/color"
	"github.com/kierdavis/dateparser"
	"github.com/mitchellh/go-homedir"
	"github.com/olebedev/when"
	"github.com/olebedev/when/rules/common"
	"github.com/olebedev/when/rules/en"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	fileDateRE = regexp.MustCompile(`(?m)(\d+\-\d+\-\d+).txt$`)
	usage      = `
<magentaB>Description</>:
	Command line journaling application.

<magentaB>General Options</>:
	<yellow>-c</> | <yellow>--config</> <lightBlueEx>[CONFIG_FILE]</> => load config from [CONFIG_FILE] instead of ~/.journalrc
	<yellow>--debug</> => turn on debug info
	<yellow>-h</> | <yellow>--help</> => print usage information

<magentaB>Entry Options</>:
	<yellow>-j</> | <yellow>--journal</> <lightBlueEx>[JOURNAL]</> => add entry to [JOURNAL]
	<yellow>--date</> <lightBlueEx>[DATE]</> => add the entry under [DATE]

<magentaB>Viewing Options</>:
	<yellow>--since</> <lightBlueEx>[DATE]</> => view entries added after or on [DATE]
	<yellow>--until</> <lightBlueEx>[DATE]</> => view entries added before or on [DATE]
	<yellow>--on</> <lightBlueEx>[DATE]</> => view entries added on [DATE]
	<yellow>-s</> | <yellow>--search</> <lightBlueEx>[SEARCH TERM]</> => view entries containing [SEARCH TERM]
	<yellow>--smartcase</> => (default:true) perform vim-style smartcase search

<magentaB>Editing Options</>:
	<yellow>-e</> | <yellow>--edit</> <lightBlueEx>[DATE]</> => edit entries added on [DATE]
`
)

type app struct {
	SelectedJournal         string
	SelectedJournalLocation string
	Config                  struct {
		ConfigFile string
		Debug      bool
		Help       bool
		SmartCase  bool
		Journal    string
		Date       string
		SinceDate  string
		UntilDate  string
		OnDate     string
		Search     string
		Edit       string
	}
}

func main() {
	a := app{}

	pflag.StringVarP(&a.Config.ConfigFile, "config", "c", "", "load config from [CONFIG_FILE] instead of ~/.journalrc.toml")
	pflag.BoolVar(&a.Config.Debug, "debug", false, "enable debug info")
	pflag.BoolVarP(&a.Config.Help, "help", "h", false, "print usage information")
	pflag.BoolVar(&a.Config.SmartCase, "smartcase", true, "enable vim style smartcase search")
	pflag.StringVarP(&a.Config.Journal, "journal", "j", "", "name of journal to add entry to")
	pflag.StringVar(&a.Config.Date, "date", "", "add the entry under [DATE]")
	pflag.StringVar(&a.Config.SinceDate, "since", "", "view entries added after or on [DATE]")
	pflag.StringVar(&a.Config.UntilDate, "until", "", "view entries added before or on [DATE]")
	pflag.StringVar(&a.Config.OnDate, "on", "", "view entries added on [DATE]")
	pflag.StringVarP(&a.Config.Search, "search", "s", "", "view entries containing [SEARCH TERM]")
	pflag.StringVarP(&a.Config.Edit, "edit", "e", "", "edit entries added on [DATE]")
	pflag.Parse()

	if a.Config.Help {
		color.Fprint(os.Stderr, usage)
		os.Exit(0)
	}

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	zerolog.SetGlobalLevel(zerolog.WarnLevel)

	if a.Config.Debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	viper.SetConfigName(".journalrc")
	viper.AddConfigPath("$HOME")

	if a.Config.ConfigFile != "" {
		viper.SetConfigFile(a.Config.ConfigFile)
	}

	if configErr := viper.ReadInConfig(); configErr != nil {
		if _, ok := configErr.(viper.ConfigFileNotFoundError); ok {
			log.Fatal().Msg("config not found")
		} else {
			log.Fatal().Err(configErr).Msg("config could not be read in")
		}
	}

	a.SelectedJournal = viper.GetString("journal.default")
	if a.Config.Journal != "" {
		a.SelectedJournal = a.Config.Journal
	}

	a.SelectedJournalLocation = viper.GetString(a.SelectedJournal + ".location")
	log.Debug().Str("name", a.SelectedJournal).Str("location", a.SelectedJournalLocation).Msg("default journal found")

	switch {
	case a.Config.SinceDate != "", a.Config.UntilDate != "", a.Config.OnDate != "", a.Config.Search != "":
		a.printEntries()
		return
	case a.Config.Edit != "":
		a.editEntry()
		return
	}

	// write the entry
	a.writeEntry(pflag.Args())
}

// printEntries will print all entries after applying any date filters the user provided
func (a *app) printEntries() {
	rootPath, _ := homedir.Expand(a.SelectedJournalLocation)

	startTime := robustParseTime(a.Config.SinceDate)
	endTime := robustParseTime(a.Config.UntilDate)
	onTime := robustParseTime(a.Config.OnDate)

	log.Debug().Str("path", rootPath).Str("onTime", onTime.String()).Msg("searching for entries to print")

	_ = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if info != nil && !info.IsDir() {
			fileDate, _ := time.Parse("2006-01-02", fileDateRE.FindStringSubmatch(info.Name())[1])

			if a.Config.OnDate != "" && !fileDate.Equal(onTime) {
				return nil
			}

			if a.Config.SinceDate != "" && (fileDate.Before(startTime) && !fileDate.Equal(startTime)) {
				if a.Config.UntilDate != "" && fileDate.After(endTime) {
					return nil
				}
				return nil
			}
			if a.Config.UntilDate != "" && (fileDate.After(endTime) && !fileDate.Equal(endTime)) {
				if a.Config.SinceDate != "" && fileDate.Before(startTime) {
					return nil
				}
				return nil
			}

			a.printFile(path)
		}
		return nil
	})
}

func (a *app) editEntry() {
	// use the user's preferred EDITOR but default to vim
	selectedEditor, ok := os.LookupEnv("EDITOR")
	if !ok {
		selectedEditor = "vim"
	}

	// open the editor
	editor, err := exec.LookPath(selectedEditor)
	if err != nil {
		log.Fatal().Msg("could not find an approriate editor")
	}

	t := robustParseTime(a.Config.Edit)

	// escape any spaces in folder names
	fPath := regexp.MustCompile(`(?m) `).ReplaceAllString(a.SelectedJournalLocation, "\\ ")

	cmd := exec.Command("bash", "-c", fmt.Sprintf("%s %s/%s.txt", editor, fPath, t.Format("2006-01-02")))
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}

func (a *app) writeEntry(entryData []string) {
	// check if the user provided an entry date
	var t time.Time
	if a.Config.Date != "" {
		t = robustParseTime(a.Config.Date)
	} else {
		t = time.Now()
	}

	rootPath, _ := homedir.Expand(a.SelectedJournalLocation)

	// escape any spaces in folder names
	dirPath := regexp.MustCompile(`(?m) `).ReplaceAllString(rootPath, "\\ ")
	fPath := fmt.Sprintf("%s/%s.txt", rootPath, t.Format("2006-01-02"))

	// write the timestamp to the file
	timestamp := t.Format("Mon 01/02/06 15:04:05")
	appendToFile(fPath, fmt.Sprintf("\n%s\n", timestamp))

	// if the entry was provided on the cmd line, write it
	if len(entryData) > 0 {
		appendToFile(fPath, "- "+strings.Join(entryData, " "))
		return
	}

	// if no entry data provided on the command line then pop open the editor

	// use the user's preferred EDITOR but default to vim
	selectedEditor, ok := os.LookupEnv("EDITOR")
	if !ok || selectedEditor == "vim" {
		selectedEditor = "vim +'normal Ga'" // open vim with the cursor at the end of the file
	}

	editPath := fmt.Sprintf("%s %s/%s.txt", selectedEditor, dirPath, t.Format("2006-01-02"))

	cmd := exec.Command("bash", "-c", editPath)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}

// helper function to write a string to a file
func appendToFile(file, data string) {
	f, err := os.OpenFile(file,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Error().Err(err).Msg("error opening file")
	}

	defer f.Close()
	_, _ = f.WriteString(data + "\n")
}

// printFile prints all entries in a file, optionally filtered by a search query
func (a *app) printFile(fileName string) {
	file, err := os.Open(fileName)
	if err != nil {
		return
	}

	content, _ := ioutil.ReadAll(file)

	if a.Config.Search == "" {
		fmt.Print(highlightTimestamps(string(content)))
	} else {
		matchRE := regexp.MustCompile(`(?ms)\w+ \d+/\d+/\d+ \d+:\d+:\d+\n(.*?)^$`)

		s := a.Config.Search

		// if requested, perform a vim-style smartcase search:
		//		case insensitive search if the query is lowercase, case sensitive if the query starts with capital letter
		if a.Config.SmartCase {
			if unicode.IsLower(rune(s[0])) {
				s = `(?i)` + s
			}
		}

		searchRE := regexp.MustCompile(s)
		for _, match := range matchRE.FindAllStringSubmatch(string(content), -1) {
			if searchRE.MatchString(match[1]) {
				fmt.Println(highlightTimestamps(match[0]))
			}
		}
	}
}

func highlightTimestamps(l string) string {
	// highlight all timestamps
	re := regexp.MustCompile(`\w+ \d+/\d+/\d+ \d+:\d+:\d+\n`)
	if re.MatchString(l) {
		timestamp := re.FindString(l)
		const escape = "\x1b"
		timestamp = fmt.Sprintf("%s[%dm", escape, 95) + timestamp + fmt.Sprintf("%s[%dm", escape, 0)
		l = re.ReplaceAllString(l, timestamp)

	}
	return l
}

// attempt parsing a datetime string using a couple of libraries
func robustParseTime(rawTime string) time.Time {
	// first try parsing with https://github.com/kierdavis/dateparser
	parser := &dateparser.Parser{}
	t, err := parser.Parse(rawTime)
	if err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}

	// if that failed then try with https://github.com/olebedev/when
	w := when.New(nil)
	w.Add(en.All...)
	w.Add(common.All...)

	r, err := w.Parse(rawTime, time.Now())
	if err == nil {
		t = r.Time.UTC()
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}

	// if both failed, return a nil time
	return time.Time{}
}
