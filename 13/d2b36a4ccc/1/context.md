# Session Context

## User Prompts

### Prompt 1

Implement the following plan:

# main.go リファクタリング計画

## Context
`main()` にビジネスロジック・共有状態・goroutine が混在して可読性が低い。動作を変えずに構造を整理する。

## 変更方針（最小限の抽象化）

### 1. `monitor` struct の導入
6つの共有変数 + `sync.Mutex` を1つの struct に集約。
`state` のゼロ値は `stateIdle`（= 0）なので、初期化は `lastOutput: time.Now()` のみ。

### 2. goroutine を...

### Prompt 2

ロジックに変更がないかどうか、チェックしてください

### Prompt 3

コミットして

### Prompt 4

ビルドして

