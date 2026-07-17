# Go Language Study — from syntax to CPU, RAM, threads & architecture

> A single-file study guide for learning Go (Golang). It goes from the language basics up to
> how Go *actually* runs on the machine: how it maps goroutines onto OS threads, how the
> scheduler shares the CPU, how memory (RAM) is laid out and garbage-collected, how it relates
> to processes — and how to structure a real Go app in clean layers.
>
> Tested against **Go 1.26** (this repo's `go.mod` uses `go 1.26.5`).

---

## Table of contents

1. [Mental model: why Go exists](#1-mental-model-why-go-exists)
2. [Language basics (fast tour)](#2-language-basics-fast-tour)
3. [Process vs thread vs goroutine](#3-process-vs-thread-vs-goroutine)
4. [The Go scheduler & the CPU (the GMP model)](#4-the-go-scheduler--the-cpu-the-gmp-model)
5. [Memory & RAM: stack, heap, escape analysis, GC](#5-memory--ram-stack-heap-escape-analysis-gc)
6. [Concurrency in practice: channels, sync, patterns](#6-concurrency-in-practice-channels-sync-patterns)
7. [The Go memory model (happens-before)](#7-the-go-memory-model-happens-before)
8. [Layered / clean architecture in Go](#8-layered--clean-architecture-in-go)
9. [Tooling to observe CPU, RAM & goroutines](#9-tooling-to-observe-cpu-ram--goroutines)
10. [Study path & exercises](#10-study-path--exercises)

---

## 1. Mental model: why Go exists

Go was built at Google to make **networked, concurrent, multi-core** server software easy to
write and fast to compile. Its three big bets:

- **Concurrency is a language feature, not a library.** `go f()` starts a lightweight
  concurrent task; `chan` passes data between tasks safely.
- **The runtime hides the OS.** You write thousands of *goroutines*; the Go runtime multiplexes
  them onto a small pool of OS threads and spreads them across CPU cores for you.
- **A garbage collector manages RAM**, tuned for *low latency* (short pauses) rather than max
  throughput — good for servers.

Key idea to hold onto: **you almost never touch threads directly in Go.** You create goroutines;
the runtime owns the threads. Everything in sections 3–5 explains that sentence.

---

## 2. Language basics (fast tour)

```go
package main

import "fmt"

// A struct = a value type grouping fields (like a C struct / a class without methods inline).
type User struct {
    ID   int
    Name string
}

// Method with a receiver. Pointer receiver (*User) can mutate; value receiver copies.
func (u *User) Rename(name string) { u.Name = name }

// Interface = a set of method signatures. Satisfied implicitly (no "implements" keyword).
type Greeter interface {
    Greet() string
}

func (u User) Greet() string { return "Hi, " + u.Name }

func main() {
    u := User{ID: 1, Name: "Ann"}
    u.Rename("Anna")

    var g Greeter = u          // User satisfies Greeter automatically
    fmt.Println(g.Greet())     // Hi, Anna

    // Slices (growable views over arrays) and maps.
    nums := []int{1, 2, 3}
    nums = append(nums, 4)
    ages := map[string]int{"ann": 30}

    // Multiple return values; error is a normal value, not an exception.
    if v, ok := ages["bob"]; ok {
        fmt.Println(v)
    }
    _ = nums
}
```

Things that surprise newcomers:

| Feature | Note |
|---|---|
| **No exceptions** | Functions return `error` as a value: `v, err := doThing()`. Handle it explicitly. `panic`/`recover` exist but are for truly exceptional cases. |
| **Implicit interfaces** | A type satisfies an interface just by having the methods. Enables loose coupling (great for the layers in §8). |
| **`defer`** | Schedules a call to run when the function returns — used for cleanup (`defer f.Close()`). |
| **Zero values** | Every type has a usable zero (`0`, `""`, `nil`, empty struct). No "uninitialized" garbage. |
| **Pointers, no pointer arithmetic** | `&x` / `*p` exist; you can't do `p+1`. Safer than C. |
| **`go` keyword** | `go f()` launches a goroutine. This is the door to everything below. |
| **Capital = exported** | `Name` is public across packages; `name` is package-private. Visibility is by case. |

---

## 3. Process vs thread vs goroutine

These three words are often confused. Precise definitions:

| Unit | Owned by | Has its own... | Cost to create | Count you'd run |
|---|---|---|---|---|
| **Process** | OS | Address space (isolated RAM), file descriptors, PID | Heavy (ms) | 1 (your Go program is one process) |
| **OS thread** | OS kernel | Stack (~1–8 MB), register set; **shares** the process's RAM | Medium (~µs–ms, ~1MB) | Dozens |
| **Goroutine** | Go runtime | Tiny growable stack (starts ~2 KB); shares the process RAM | Very cheap (~ns, ~2KB) | **Thousands to millions** |

```
┌─────────────────────────── OS Process (your Go binary) ───────────────────────────┐
│  One shared address space (heap, globals, code)                                     │
│                                                                                     │
│   OS thread M1        OS thread M2        OS thread M3     ← the OS schedules THESE  │
│   ┌──────────┐        ┌──────────┐        ┌──────────┐       onto CPU cores         │
│   │ G3 G7 G8 │        │ G1 G4    │        │ G2 G5 G9 │     ← goroutines the GO       │
│   └──────────┘        └──────────┘        └──────────┘       runtime schedules      │
└─────────────────────────────────────────────────────────────────────────────────┘
```

So the layering is: **CPU cores ← OS threads ← goroutines**. The OS only ever sees and schedules
threads. Go's runtime does a *second* layer of scheduling, packing many goroutines onto each
thread. This is called **M:N scheduling** (M goroutines on N threads).

**Why goroutines are cheap:**
- Their stack starts at ~2 KB and **grows/shrinks on demand** (the runtime copies the stack to a
  bigger chunk when needed). An OS thread reserves ~1 MB up front.
- Switching between goroutines happens in *user space* — no expensive kernel context switch when
  the runtime hands the same thread to a different goroutine.

**Processes in Go:** you rarely fork. To run another program you use `os/exec`:

```go
out, err := exec.Command("git", "status").Output() // spawns a child OS process
```

That child is a *separate process* with its own isolated RAM — no shared memory, communicate via
pipes/stdout (exactly what this repo's `skillrunner` does when it shells out).

---

## 4. The Go scheduler & the CPU (the GMP model)

The runtime scheduler is built from three entities. Learn these letters — all the Go concurrency
docs use them:

| Letter | Meaning | Analogy |
|---|---|---|
| **G** | Goroutine — a task + its stack + its state | A job to do |
| **M** | Machine — an **OS thread** (the thing the OS puts on a CPU core) | A worker |
| **P** | Processor — a **scheduling context** holding a run-queue of Gs + resources an M needs to run Go code | A workbench with a to-do list |

**The rule: an M must hold a P to execute Go code.** The number of Ps = `GOMAXPROCS`, which by
default equals the number of CPU cores. So **`GOMAXPROCS` is the real limit on how many
goroutines run *in parallel*** at any instant.

```
GOMAXPROCS = 4   → 4 Ps → at most 4 goroutines running Go code simultaneously

   P0 ── M2 ──▶ CPU core        each P has a local run-queue of ready goroutines
   P1 ── M0 ──▶ CPU core
   P2 ── M5 ──▶ CPU core        + one GLOBAL run-queue shared by all Ps
   P3 ── M1 ──▶ CPU core

   Idle Ms parked, ready to be grabbed when a blocking call needs a fresh thread.
```

Concurrency vs parallelism (Go's most quoted distinction):
- **Concurrency** = *dealing with* many things (structure): you can have 100,000 goroutines.
- **Parallelism** = *doing* many things at once (execution): limited to `GOMAXPROCS` cores.

### How a goroutine gets CPU time

1. `go f()` creates a G and pushes it onto the current P's local run-queue.
2. When the running G blocks or yields, the P pops the next G from its queue and runs it on its M.
3. **Work stealing:** if a P's queue is empty, it steals half the Gs from another P's queue —
   this keeps all cores busy and balanced.
4. **Preemption:** since Go 1.14 the scheduler is *asynchronously preemptive* — a goroutine that
   runs too long (~10 ms) is interrupted via a signal so it can't hog a core. (Before 1.14 a tight
   loop with no function calls could starve everyone.)

### Blocking: the clever part

- **Blocking on a channel / mutex / `time.Sleep`:** the goroutine is *parked* (removed from the
  thread) and the M immediately runs another G. Cost is tiny — no thread blocked.
- **Blocking on a *syscall*** (e.g. reading a file): the M is stuck in the kernel, so the runtime
  **detaches the P from that M** and hands the P to another M (creating/reusing one) so the other
  goroutines keep running. When the syscall returns, the M tries to get a P back.
- **Network I/O** is special: Go's **netpoller** uses the OS's async facility (epoll/kqueue/IOCP).
  A goroutine doing `conn.Read` *looks* blocking to you, but under the hood it's parked and woken
  by the netpoller when data arrives — so one thread serves thousands of network connections.

This is why a Go web server can handle huge concurrency with a handful of threads.

### Tuning it

```go
import "runtime"

runtime.GOMAXPROCS(4)      // or set env GOMAXPROCS=4
n := runtime.NumCPU()      // logical cores available
g := runtime.NumGoroutine()// live goroutine count (great for leak detection)
```

Go 1.25+ is also **cgroup-aware**: in a container with a CPU limit, `GOMAXPROCS` defaults to the
limit, not the host's core count — important for Kubernetes.

---

## 5. Memory & RAM: stack, heap, escape analysis, GC

A Go process's RAM is split like any native program, plus a runtime-managed heap:

```
 high addr ┌──────────────┐
           │    Stack(s)   │  one per goroutine, grows down, tiny & fast (auto-freed on return)
           │      ↓        │
           │      ↑        │
           │     Heap      │  runtime-managed, grows up, holds anything that "escapes"; GC'd
           ├──────────────┤
           │  BSS / Data   │  globals (zeroed / initialized)
           │     Text      │  the compiled machine code
 low addr  └──────────────┘
```

### Stack vs heap — who decides?

You do **not** choose with `new` vs `&` the way C++ does. The **compiler's escape analysis**
decides at build time:

- If a value's lifetime clearly ends with the function → it lives on the **stack** (free, instant
  cleanup when the function returns).
- If a reference to it *escapes* (returned, stored in a global, captured by a goroutine, put in an
  interface) → it's moved to the **heap**, where the GC manages it.

```go
func onStack() int {
    x := 42          // stays on stack — never escapes
    return x         // the value is copied out
}

func onHeap() *int {
    x := 42          // ESCAPES: we return its address, so it must outlive the call
    return &x        // → allocated on the heap; the GC will free it later
}
```

See the decision with:

```bash
go build -gcflags='-m' ./...      # prints "escapes to heap" / "does not escape"
```

Fewer heap allocations = less GC work = faster, lower-RAM programs. This is the #1 Go performance
lever.

### The garbage collector (GC)

- Go uses a **concurrent, tri-color, mark-and-sweep** collector. "Concurrent" = it runs *alongside*
  your goroutines, so **stop-the-world pauses are sub-millisecond**, not proportional to heap size.
- It's tuned for **low latency**, which suits servers (predictable response times) over raw batch
  throughput.

**When does it run?** Controlled by two knobs:

| Knob | What it does | Default |
|---|---|---|
| `GOGC` (env or `debug.SetGCPercent`) | Trigger GC when the heap has grown this % since the last GC. `GOGC=100` = collect when heap doubles. Higher = fewer GCs, more RAM. | 100 |
| `GOMEMLIMIT` (Go 1.19+) | A soft **memory cap**. The GC works harder as you approach it — prevents OOM in containers. | off |

```go
import "runtime/debug"

debug.SetGCPercent(200)                 // trade RAM for fewer collections
debug.SetMemoryLimit(512 << 20)         // 512 MiB soft limit
```

**Reading live memory stats:**

```go
var m runtime.MemStats
runtime.ReadMemStats(&m)
fmt.Printf("HeapAlloc=%d MB  NumGC=%d  Goroutines=%d\n",
    m.HeapAlloc/1024/1024, m.NumGC, runtime.NumGoroutine())
```

### Practical RAM tips

- **Reuse buffers** with `sync.Pool` for hot paths (reduces allocations → less GC).
- **Pre-size** slices/maps: `make([]T, 0, n)` avoids repeated re-allocation + copying as they grow.
- **Pass big structs by pointer**, small ones by value (copies are cheap and stay on the stack).
- A **goroutine leak** is the classic Go memory leak: a goroutine blocked forever on a channel is
  never collected (nor is anything it references). Watch `runtime.NumGoroutine()`.

---

## 6. Concurrency in practice: channels, sync, patterns

Go's motto: **"Don't communicate by sharing memory; share memory by communicating."** Prefer
channels to pass ownership of data; fall back to `sync` primitives for simple shared state.

### Goroutines + channels

```go
// A channel is a typed, thread-safe conduit between goroutines.
ch := make(chan int)        // unbuffered: send blocks until a receiver is ready (a handshake)
buf := make(chan int, 100)  // buffered: send blocks only when full

go func() {                 // producer goroutine
    for i := 0; i < 5; i++ {
        ch <- i             // send
    }
    close(ch)               // tell receivers no more values are coming
}()

for v := range ch {         // receive until the channel is closed
    fmt.Println(v)
}
```

### `select` — wait on multiple channels

```go
select {
case v := <-in:
    handle(v)
case out <- result:
    // sent
case <-time.After(time.Second):
    // timeout
case <-ctx.Done():
    return ctx.Err()        // cancellation (see below)
}
```

### `context` — the standard way to cancel & set deadlines

Threading a `context.Context` through your call stack lets you cancel a whole tree of goroutines
(e.g. when an HTTP client disconnects) and enforce timeouts. This is idiomatic in every real Go
server; make it the first parameter of functions that do I/O.

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()
result, err := slowCall(ctx) // slowCall watches ctx.Done() and bails out early
```

### The `sync` toolbox (shared-memory side)

| Primitive | Use for |
|---|---|
| `sync.Mutex` / `RWMutex` | Protect a shared variable from concurrent access (data races) |
| `sync.WaitGroup` | Wait for a batch of goroutines to finish |
| `sync.Once` | Run initialization exactly once (thread-safe singletons) |
| `atomic.*` (`sync/atomic`) | Lock-free counters/flags (`atomic.Int64`, etc.) |
| `sync.Pool` | Reuse temporary objects to cut allocations |

```go
var wg sync.WaitGroup
for _, job := range jobs {
    wg.Add(1)
    go func(j Job) {
        defer wg.Done()
        process(j)
    }(job)          // pass j as an arg to avoid the classic loop-variable capture bug
}
wg.Wait()           // block until all goroutines call Done()
```

> Note: since **Go 1.22** each loop iteration gets its own copy of the loop variable, so the
> capture bug above is fixed — but passing args explicitly is still the clearest habit.

### Worker pool (bound concurrency to CPU)

```go
jobs := make(chan Job)
results := make(chan Result)

for i := 0; i < runtime.NumCPU(); i++ {   // N workers = N cores
    go func() {
        for j := range jobs {             // each worker pulls jobs until channel closes
            results <- process(j)
        }
    }()
}
```

This caps parallelism at core count and reuses goroutines — the standard pattern for CPU-bound
fan-out.

### Detecting bugs

```bash
go test -race ./...     # the race detector: finds concurrent unsynchronized access
go vet ./...            # catches common concurrency & correctness mistakes
```

Always run `-race` in CI. A data race is undefined behavior even in Go.

---

## 7. The Go memory model (happens-before)

To reason about concurrent code you need to know *when one goroutine is guaranteed to see another
goroutine's writes*. Go defines this with **happens-before** relationships. The essentials:

- Within a single goroutine, statements happen in program order.
- **A send on a channel happens-before the corresponding receive completes.** (Channels create
  ordering — this is why passing data via channels is safe without extra locks.)
- **A `Mutex.Unlock` happens-before the next `Lock`.**
- `sync.Once`, `WaitGroup.Wait`, and `atomic` operations also establish ordering.

If two goroutines touch the same variable and *at least one writes*, and there is **no**
happens-before between them, that's a **data race** → undefined behavior. Fix it with a channel,
a mutex, or an atomic. Never rely on "it seems to work" — the compiler and CPU are free to reorder
unsynchronized memory operations.

---

## 8. Layered / clean architecture in Go

Go has no framework-imposed structure, so you impose one. The dominant pattern for services is
**layered / hexagonal (ports & adapters) architecture**, enforced by Go's implicit interfaces and
package boundaries.

**The dependency rule: dependencies point *inward*. Inner layers know nothing about outer ones.**

```
        ┌──────────────────────────────────────────────────────┐
        │  transport / handler  (HTTP, gRPC, CLI)                │  ← outermost
        │    parses requests, calls a use-case, writes responses │
        │        │ depends on ↓                                   │
        │  ┌───────────────────────────────────────────────┐    │
        │  │  service / use-case (business logic)           │    │
        │  │    orchestrates domain rules; defines the      │    │
        │  │    interfaces (ports) it needs                 │    │
        │  │        │ depends on ↓ (an INTERFACE, not impl) │    │
        │  │  ┌─────────────────────────────────────────┐  │    │
        │  │  │  domain / entity (pure types + rules)    │  │    │  ← innermost, no deps
        │  │  └─────────────────────────────────────────┘  │    │
        │  └───────────────────────────────────────────────┘    │
        │  repository / adapter (Postgres, Redis, HTTP client)   │
        │    IMPLEMENTS the interfaces the service defined        │
        └──────────────────────────────────────────────────────┘
```

Typical package layout:

```
myapp/
├── cmd/
│   └── server/main.go        # composition root: wire everything together, start the process
├── internal/                 # not importable by other modules — your private code
│   ├── domain/               # entities + business rules, zero external deps
│   │   └── user.go
│   ├── service/              # use-cases; declares interfaces it needs (ports)
│   │   └── user_service.go
│   ├── repository/           # adapters: DB/cache implementations of those interfaces
│   │   └── user_postgres.go
│   └── transport/http/       # handlers, routing, (de)serialization
│       └── user_handler.go
└── go.mod
```

### The key move: define interfaces where they're *used*, not where they're implemented

```go
// internal/service/user_service.go  — the INNER layer declares what it needs
package service

type UserRepo interface {                 // a "port"
    FindByID(ctx context.Context, id int) (*domain.User, error)
    Save(ctx context.Context, u *domain.User) error
}

type UserService struct{ repo UserRepo }  // depends on the interface, not Postgres

func New(r UserRepo) *UserService { return &UserService{repo: r} }

func (s *UserService) Rename(ctx context.Context, id int, name string) error {
    u, err := s.repo.FindByID(ctx, id)
    if err != nil { return err }
    u.Rename(name)                        // domain rule
    return s.repo.Save(ctx, u)
}
```

```go
// internal/repository/user_postgres.go — the OUTER layer implements it
package repository

type PostgresUserRepo struct{ db *sql.DB }

func (r *PostgresUserRepo) FindByID(ctx context.Context, id int) (*domain.User, error) { /* SQL */ }
func (r *PostgresUserRepo) Save(ctx context.Context, u *domain.User) error             { /* SQL */ }
// It satisfies service.UserRepo implicitly — no "implements" needed.
```

```go
// cmd/server/main.go — the composition root wires concrete → abstract
func main() {
    db := openDB()
    repo := &repository.PostgresUserRepo{db}      // concrete
    svc := service.New(repo)                       // inject via the interface
    h := httptransport.NewUserHandler(svc)
    http.ListenAndServe(":8080", h.Routes())
}
```

Why this pays off:

- **Testable:** in a unit test, pass a fake `UserRepo` — no database needed.
- **Swappable:** replace Postgres with Redis by writing a new adapter; the service is untouched.
- **`internal/`** physically prevents other modules from importing your guts (enforced by the
  compiler).
- **Dependency injection is just passing arguments** — Go needs no DI framework.

> This is exactly the shape used by tools like the `skillrunner` in this repo: `cmd/` holds the
> entrypoint, `internal/` holds logic that shouldn't be imported externally.

---

## 9. Tooling to observe CPU, RAM & goroutines

Go ships world-class introspection. Learn these — they make the abstract sections above concrete.

```bash
# Benchmarks with allocation stats (ns/op, B/op, allocs/op)
go test -bench=. -benchmem ./...

# CPU & memory profiles → open interactively or as a flame graph
go test -cpuprofile cpu.out -memprofile mem.out -bench=.
go tool pprof cpu.out            # then: top, list <func>, web

# Escape analysis (stack vs heap decisions)
go build -gcflags='-m' ./...

# Race detector
go test -race ./...

# Execution tracer: see goroutine scheduling, GC, syscalls on a timeline
go test -trace trace.out ./...
go tool trace trace.out
```

**Live pprof in a running server** (add to any service):

```go
import _ "net/http/pprof"           // registers handlers under /debug/pprof/
go func() { http.ListenAndServe("localhost:6060", nil) }()
```

Then, while it runs:

```bash
go tool pprof http://localhost:6060/debug/pprof/heap        # RAM usage by call site
go tool pprof http://localhost:6060/debug/pprof/profile     # 30s CPU profile
curl http://localhost:6060/debug/pprof/goroutine?debug=1    # stack of every goroutine (leak hunting)
```

Useful runtime env vars to *watch the runtime think*:

```bash
GODEBUG=gctrace=1 ./app      # print a line on every GC: heap sizes + pause times
GODEBUG=schedtrace=1000 ./app# every 1000ms, print scheduler state (Ps, Ms, run-queue lengths)
GOMAXPROCS=2 ./app           # pin parallelism to 2 cores
GOGC=200 GOMEMLIMIT=512MiB ./app
```

---

## 10. Study path & exercises

A suggested order, each step building on the last:

1. **Syntax & types** — do the [Tour of Go](https://go.dev/tour/). Write structs, slices, maps,
   interfaces, error handling.
2. **Goroutines & channels** — write a program that fans work out to N goroutines and collects
   results. Then break it: cause a deadlock, cause a goroutine leak, and detect each.
3. **The scheduler** — run something CPU-bound with `GOMAXPROCS=1` then `=8`; measure the
   difference. Turn on `GODEBUG=schedtrace=1000` and read the output.
4. **Memory** — use `go build -gcflags='-m'` to make a value escape and then keep it on the stack.
   Watch `GODEBUG=gctrace=1` while allocating in a loop; then add `sync.Pool` and see GC drop.
5. **Correctness** — write a data race on purpose, catch it with `go test -race`, fix it with a
   mutex, then again with a channel.
6. **Architecture** — build a tiny service in the `cmd/ internal/{domain,service,repository,
   transport}` layout from §8. Unit-test the service with a fake repo.
7. **Profiling** — add `net/http/pprof`, load-test it, and read a CPU + heap profile in
   `go tool pprof`.

### Recap of the systems view

| You write | The runtime does | The OS does | The CPU does |
|---|---|---|---|
| `go f()` (a **G**) | Queues it on a **P**, packs many Gs onto few **M**s (threads), work-steals, preempts, parks blocked Gs, wakes them via the netpoller | Schedules **M** threads onto cores; handles syscalls | Runs ≤ `GOMAXPROCS` goroutines truly in parallel |
| `x := &T{}` | Escape analysis → stack or heap; concurrent GC reclaims heap by `GOGC`/`GOMEMLIMIT` | Maps virtual pages to physical RAM | — |
| channel / mutex | Establishes happens-before ordering so writes are visible | — | — |

### Canonical references

- **The Tour of Go** — https://go.dev/tour/
- **Effective Go** — https://go.dev/doc/effective_go
- **The Go Memory Model** — https://go.dev/ref/mem
- **Go blog: "Concurrency is not parallelism"** — https://go.dev/blog/waza-talk
- **Scheduler deep-dive** — Ardan Labs "Scheduling in Go" series
- **pprof guide** — https://go.dev/blog/pprof
```
