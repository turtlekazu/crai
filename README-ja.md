| [English](README.md) | 日本語 |
|:---:|:---:|

# crai (catcher in the rAI)

---

```
   ______  ____    ___     ____
  / ____/ / __ \  /   |   /  _/
 / /     / /_/ / / /| |   / /
/ /___  / _, _/ / ___ | _/ /
\____/ /_/ |_| /_/  |_|/___/
 catcher in the rAI
```

AI CLI が注意を引くべきタイミングで、音とバナーで通知する CLI ツールです。対応 CLI にはネイティブな Hook/notify をインストールでき、現時点では Codex と Claude Code をサポートしています。従来の PTY ラッパーモードも残しています。

半導体の畑の捕手のように —— もっとも、崖の淵で捕まえているのは、無邪気に走り回るAIエージェントであり、同時に、別のコンテキストの深みへと落ちていきそうなあなた自身の意識なのかもしれません。

> *「ぼくはこうしようと思った。耳が聞こえないふりをするんだ……AIが考え終わるまで。」*

---

## Hook モード

まず一度だけ対応 CLI に通知コマンドを設定します。

```sh
crai install claude
crai install codex
```

以後は普通に `claude` や `codex` を使うだけです。

- `crai install claude` は `~/.claude/settings.json` の `Stop` hook に command hook を追加します
- `crai install codex` は `~/.codex/config.toml` に `notify` を書き込みます

どちらも最終的には次のコマンドが呼ばれます。

```sh
crai notify --source <agent>
```

補助コマンド：

```sh
crai status claude
crai status codex
crai uninstall claude
crai uninstall codex
```

すでに `~/.codex/config.toml` に `crai` 以外の `notify` が入っている場合、`crai install codex` は上書きせずに停止します。

## 従来の PTY モード

```
 +----------+   raw stdin   +-------------+   PTY   +-----------+
 |  You     | ------------> |    crai     | ------> |  claude   |
 |  (human) | <------------ |  (watcher)  | <------ |  (AI CLI) |
 +----------+   raw stdout  +------+------+         +-----------+
                                   |
                   AI output >= 1500ms silence
                                   |
                                   v
                    * Play Sound
                    * Notification Center Banner
                    * Terminal Bell (\a)
```

1. コマンドを**疑似端末（PTY）**の中で起動します
2. raw な stdin/stdout を変換なしでブリッジします
3. 出力ストリームの**沈黙**を監視し——1500ms 以上新しい出力がなければ AI が完了したと判断します
4. 完了時：サウンド、通知センターバナー、ターミナルベルの3つを同時に発火します

### スマートフィルタリング

- **1対1プロンプト対応** — Enter を押すたびに通知が1つだけ予約されます。対応するプロンプトのない AI 出力（起動時のバナー、自発的な出力）は無視されます
- **エコー除去** — キー入力から 100ms 以内に届いた出力は PTY のエコーとして扱い、AI 出力とみなしません
- **短時間応答の抑制** — AI が 5 秒以内に応答した場合は通知しません（まだ画面を見ているものと判断します）
- **入力中の抑制** — 次のメッセージを作成中は通知しません

---

## インストール

```sh
brew install turtlekazu/tap/crai
```

ソースからビルドする場合：

```sh
git clone https://github.com/turtlekazu/crai
cd crai
go build -o crai .
sudo mv crai /usr/local/bin/
```

## アンインストール

```sh
brew uninstall crai
brew untap turtlekazu/tap
```

---

## 使い方

### Codex

```sh
crai install codex
codex
```

### Claude Code

```sh
crai install claude
claude
```

### 従来のラッパー

```sh
# claude を直接ラップする
crai claude

# 引数はそのまま透過される
crai claude --dangerously-skip-permissions
```

すべてそのまま透過されます。色、スピナー、キーバインド——すべて無傷で動作します。`crai` は必要な時まで存在感を主張しません。

---

## エイリアス設定

シェルの設定ファイル（`~/.zshrc` または `~/.bashrc`）に以下を追加してください：

```sh
alias claude="crai claude "
```

> **末尾のスペースについて**
> bash と zsh では、エイリアスの値の末尾にスペースがあると、次の単語もエイリアスとして展開されます。これにより、`claude` の後に渡す引数もエイリアス展開の対象となり、エイリアスチェーンが完全に機能します。

設定後は、いつも通り `claude` をご利用いただけます。`crai` は静かに見守っています。

---

## オプション

| フラグ | 説明 |
|--------|------|
| `--no-banner` | 通知センターバナーを無効化します |
| `--no-sound` | サウンドを無効化します |
| `--sound <path>` | 完了時に再生するサウンドファイルのパス（デフォルト: `Glass.aiff`） |
| `--silence <ms>` | 通知を発火するまでの沈黙時間（ミリ秒、デフォルト: `1500`） |

macOS には `/System/Library/Sounds/` に以下のサウンドが収録されています：

```
Basso  Blow  Bottle  Frog  Funk  Glass  Hero
Morse  Ping  Pop     Purr  Sosumi  Submarine  Tink
```

```sh
crai --sound /System/Library/Sounds/Ping.aiff claude
```

`.aiff` や `.mp3` など任意のファイルを指定することも可能です。

---

## 動作環境

- macOS（音声に `afplay`、通知センターに `terminal-notifier` を使用）
- 任意のコマンドライン AI ツール（Claude Code, Codex, Gemini CLI など）

### 通知の設定（推奨）

安定した通知のために、[`terminal-notifier`](https://github.com/julienXX/terminal-notifier) のインストールを推奨します。システム設定への正式な登録と、おやすみモードの制御が正しく機能します：

```sh
brew install terminal-notifier
```

`terminal-notifier` がインストールされていない場合は `osascript` にフォールバックします。ただし `osascript` による通知はシステム設定の通知一覧で「スクリプトエディタ」として登録されるため、設定項目が見つけにくいという問題があります。

`terminal-notifier` をインストールすると、初回の通知時に**システム設定 → 通知 → terminal-notifier** として登録され、そこから動作を調整できます。

---

## ライセンス

[MIT](LICENSE.md).

---

*「誰にも何も話すな。話したら、みんなが恋しくなるから。」*
— J.D. サリンジャー、『ライ麦畑でつかまえて』
