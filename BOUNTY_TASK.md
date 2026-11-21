# Bounty Fix

**Reward:** $10.00
**Issue:** https://github.com/Kristywvs22/chi/issues/1
**Repo:** https://github.com/Kristywvs22/chi

---

## Issue

🎯 Bug: URL parameters leak between requests due to incomplete RouteContext reset in sync.Pool

## 📝 Description
Under high-concurrency traffic with nested subrouters, URL parameters from previous requests can leak into subsequent, unrelated requests. This behavior stems from the reuse of `RouteContext` objects via the internal `sync.Pool` mechanism in `chi`. 

When a `Context` is recycled and returned to the pool, its internal routing state—specifically the `URLParams` (keys and values slices)—is not fully or safely reset. If a subsequent request processed by the same goroutine/context pool allocation does not define or overwrite the same number of URL parameters, the stale parameters from the previous request remain accessible in the slice capacity or via nested subrouter context chains.

This issue is intermittent, highly visible under load, and poses a severe security risk (e.g., tenant isolation bypass, unauthorized resource access, or data corruption due to incorrect ID lookups).

## 🎯 Acceptance Criteria
- [ ] **Complete State Reset**: The `Context.Reset()` method in `context.go` must guarantee that all routing parameters (`URLParams.Keys`, `URLParams.Values`, and any internal route path state) are completely cleared and truncated.
- [ ] **No Parameter Bleed**: Ensure that nested subrouters creating child contexts or mutating parent contexts do not leave dirty parameter states in the pooled context.
- [ ] **Zero-Allocation Safety**: The fix must not introduce unnecessary heap allocations during context reset/reuse, preserving `chi`'s high-performance characteristics.
- [ ] **Thread Safety**: The context recycling mechanism must remain safe under concurrent execution.
- [ ] **Regression Test Suite**: A concurrent test suite must be added to verify that high-volume, nested routing requests do not leak parameters across request boundaries.

## 🛠️ Technical Specifications & Context
The issue resides within the context management of the router, primarily in:
- `context.go`: Look at the `Context` struct, `RouteParams` struct, and the `Reset()` method.
- `mux.go`: Look at how `RouteContext(r.Context())` is retrieved, updated, and released back to the pool (e.g., `contextPool.Put(x)`).

### Potential Root Cause Analysis:
In `context.go`, `RouteParams` is defined as:
```go
type RouteParams struct {
	Keys   []string
	Values []string
}
```
When `Context.Reset()` is called, it typically truncates the slices:
```go
c.URLParams.Keys = c.URLParams.Keys[:0]
c.URLParams.Values = c.URLParams.Values[:0]
```
However, if nested subrouters or inline middlewares clone the context or manually manipulate the slices without resetting the underlying array references, references to old parameter values can persist. Furthermore, if `RouteContext` is pooled but nested contexts retain references to the pooled `RouteContext`'s slices, modifications can leak.

We need to ensure that:
1. `Context.Reset()` thoroughly clears all slice elements to prevent memory/string retention (e.g., setting elements to zero-values/nil before truncating).
2. The logic in `mux.go` that handles subrouter execution and context resetting correctly handles the lifecycle of nested contexts.

## 🧪 Verification & Testing
To verify the fix, Hunter (the developer) should implement a concurrent integration test in `mux_test.go`:

1. **Setup a nested router structure**:
   ```go
   r := chi.NewRouter()
   r.Route("/accounts/{accountId}", func(r chi.Router) {
       r.Get("/users/{userId}", func(w http.ResponseWriter, req *http.Request) {
           // Handler logic
       })
   })
   r.Get("/public/status", func(w http.ResponseWriter, req *http.Request) {
       // This route has NO parameters
   })
   ```
2. **Simulate high concurrency**:
   Use `sync.WaitGroup` to spin up 100+ goroutines.
   - Half of the goroutines request `/accounts/123/users/456`.
   - The other half request `/public/status`.
3. **Assertions**:
   In the `/public/status` handler, assert that `chi.URLParam(r, "accountId")` and `chi.URLParam(r, "userId")` are always empty strings (`""`).
4. **Execution**:
   Run the tests with the race detector enabled:
   ```bash
   go test -v -race -run=TestRouteContextLeak
   ```

---

[![Opire Bounty](https://img.shields.io/badge/bounty-Opire-emerald?style=for-the-badge)](https://github.com/Kristywvs22/chi)
<br/>
<hr/>

<details><summary>This repo is using Opire - what does it mean? 👇</summary><br/>💵 Everyone can add rewards for this issue commenting <code>/reward 100</code> (replace <code>100</code> with the amount).<br/>🕵️‍♂️ If someone starts working on this issue to earn the rewards, they can comment <code>/try</code> to let everyone know!<br/>🙌 And when they open the PR, they can comment <code>/claim #1</code> either in the PR description or in a PR's comment.<br/><br/>🪙 Also, everyone can tip any user commenting <code>/tip 20 @Kristywvs22</code> (replace <code>20</code> with the amount, and <code>@Kristywvs22</code> with the user to tip).<br/><br/>📖 If you want to learn more, check out our <a href="https://docs.opire.dev">documentation</a>.</details>

---

## Process

### 1. Orient yourself first
Before touching any code:
- Read the README and any relevant docs
- Run: find . -type f | grep -v node_modules | grep -v .git | grep -v dist | head -60
- Grep for the key terms from the issue title to find the relevant files
- Read the files around the bug — understand what's currently happening

### 2. Find and read the tests
- Look for test files near the relevant code (*.test.ts, *.spec.js, __tests__, etc.)
- Read them — they tell you what behavior is expected
- Understand what a passing fix looks like before writing a line

### 3. Implement the fix
- Make the MINIMAL change that fixes the issue — nothing more
- Do not refactor, rename, reorganize, or clean up unrelated code
- Do not add dependencies unless the issue explicitly requires it
- Do not modify config files (package.json, tsconfig.json, etc.) unless that IS the fix
- One logical change, one commit

### 4. Verify
- Run the test suite: check package.json scripts for "test", "lint", "typecheck"
  Common commands: npm test, yarn test, pnpm test, pytest, go test ./..., cargo test
- If TypeScript is present: npx tsc --noEmit
- Fix any test failures your change caused — if a test was wrong before, update it

### 5. Commit and document
- git add -A
- git commit -m "fix: 🎯 Bug: URL parameters leak between requests due to incomplete RouteCont"
- Write AGENT_SUMMARY.md with:
  - Root cause: what was wrong and why
  - Fix: what you changed and the reasoning
  - Verification: how to confirm it works
  - Concerns: anything a reviewer should know

---

## Stop conditions

If ANY of these are true, write "UNCERTAIN: [reason]" at the top of AGENT_SUMMARY.md, make NO code changes, and stop:
- The fix requires changing more than ~5 files
- You cannot determine what the correct behavior should be
- The issue requires a database migration, API key, or external service
- You find 3+ open PRs already attempting this fix (high competition)
- The repo is not in a language/framework you can reason about confidently

A clean UNCERTAIN is better than a bad PR.
