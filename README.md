A Golang command line journaling application that combines the best features from [journal](https://github.com/askedrelic/journal) & [jrnl](https://pypi.python.org/pypi/jrnl) plus several more features.

# Installation

Install the program:

```sh
go get github.com/jakewarren/journal
```

Create a .journalrc.toml config file in your home directory. 

##### Minimal .journalrc file with single journal:

```
[journal]
default=personal

[personal]
location="~/.journal"

```

##### Sample .journalrc file with multiple journals:

```
[journal]
default="work"

[personal]
location="~/SpiderOak Hive/journals/personal"

[work]
location="~/SpiderOak Hive/journals/work"
```

# Usage

Date values ([DATE]) are flexible and can be "human-readable" such as 'today','yesterday','1pm 2 days ago', etc.

The editor used for editing entries respects your EDITOR environment variable but defaults to vim.

```
Description:
	Command line journaling application.

General Options:
	-c | --config [CONFIG_FILE] => load config from [CONFIG_FILE] instead of ~/.journalrc
	--debug => turn on debug info
	-h | --help => print usage information

Entry Options:
	-j | --journal [JOURNAL] => add entry to [JOURNAL]
	--date [DATE] => add the entry under [DATE]

Viewing Options:
	--since [DATE] => view entries added after or on [DATE]
	--until [DATE] => view entries added before or on [DATE]
	--on [DATE] => view entries added on [DATE]
	-s | --search [SEARCH TERM] => view entries containing [SEARCH TERM]
	--smartcase => (default:true) perform vim-style smartcase search

Editing Options:
	-e | --edit [DATE] => edit entries added on [DATE]
```

