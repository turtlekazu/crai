# Session Context

## User Prompts

### Prompt 1

brew tap turtlekazu/tapの後にbrew install craiをするのと、brew install turtlekazu/tap/craiをするのはどう違うの？

### Prompt 2

brew tap後、tapを外すにはどうしますか？

### Prompt 3

<bash-input>brew uninstall crai</bash-input>

### Prompt 4

<bash-stdout>Uninstalling /opt/homebrew/Cellar/crai/0.0.1... (6 files, 1.9MB)</bash-stdout><bash-stderr></bash-stderr>

### Prompt 5

<bash-input>brew untap turtlekazu/tap</bash-input>

### Prompt 6

<bash-stdout>Untapping turtlekazu/tap...
Untapped 1 formula (16 files, 8.3KB).</bash-stdout><bash-stderr></bash-stderr>

### Prompt 7

brew install crai

### Prompt 8

[Request interrupted by user]

### Prompt 9

<bash-input>brew install crai</bash-input>

### Prompt 10

<bash-stdout>Warning: No available formula with the name "crai". Did you mean cai or cram?
==> Searching for similarly named formulae and casks...
==> Formulae
cai
cram

To install cai, run:
  brew install cai

==> Casks
chai

To install chai, run:
  brew install --cask chai
</bash-stdout><bash-stderr>Warning: No available formula with the name "crai". Did you mean cai or cram?
==> Searching for similarly named formulae and casks...
==> Formulae
cai
cram

To install cai, run:
  brew install cai
...

### Prompt 11

<bash-input>brew install turtlekazu/tap/crai</bash-input>

### Prompt 12

<bash-stdout>==> Tapping turtlekazu/tap
Cloning into '/opt/homebrew/Library/Taps/turtlekazu/homebrew-tap'...
Tapped 1 formula (15 files, 7KB).
==> Fetching downloads for: crai
✔︎ Formula crai (0.0.1)
==> Installing crai from turtlekazu/tap
[34m==>[0m [1mgo build -ldflags=-s -w[0m
🍺  /opt/homebrew/Cellar/crai/0.0.1: 6 files, 1.9MB, built in 1 second
==> Running `brew cleanup crai`...
Disable this behaviour by setting `HOMEBREW_NO_INSTALL_CLEANUP=1`.
Hide these hints with `HOMEBREW_NO_...

### Prompt 13

<bash-input>brew untap turtlekazu/tap</bash-input>

### Prompt 14

<bash-stdout>Error: Refusing to untap turtlekazu/tap because it contains the following installed formulae or casks:
crai
</bash-stdout><bash-stderr>Error: Refusing to untap turtlekazu/tap because it contains the following installed formulae or casks:
crai
</bash-stderr>

### Prompt 15

タップは複数にタップした状態になれますか？たとえば、別の個人ツールをtapしてインストールしたい場合など

### Prompt 16

干渉しないんですね。READMEに、uninstall方法も載せといた方がいいですかね？

### Prompt 17

それを追加して

### Prompt 18

コミット

