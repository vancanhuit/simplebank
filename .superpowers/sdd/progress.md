# SimpleBank SDD Progress Ledger

Plan: docs/superpowers/plans/2026-07-14-simplebank-cloud-native.md
Branch: feat/simplebank-implementation
Base commit: bc41bab

## Task status
Task 1: complete (commits bc41bab..da36896, review clean)
Task 2: complete (commits da36896..8fa0709, review clean)
Task 3: complete (commits 8fa0709..304878e, review clean)
Task 4: complete (commits 304878e..be6a9cd, review clean)
Task 5: complete (commits be6a9cd..4ef3f0d, review clean) [sqlc v1.31.1; ClientIp, Limit/Offset int32]
Task 6+7: complete (commit 61a7bf6, review clean) [store, execTx, TransferTx combined]
Task 8: complete (commit c88bdd5, review clean) [integration tests pass -race; OpenDBFromPool works]
Task 9: complete (commit ba4a379, review clean) [JWT maker; security-audited]
Task 10: complete (commit ab5832e, review clean) [go-mail v0.8.1]
Task 11: complete (commits ab5832e..cb88c21, review clean) [River v0.40.0; crypto secret_code; html-escape fix cb88c21]
Task 12: complete (commit 982ee86, review clean) [api scaffold; UnwrapResponse committed-check]
Task 13: complete (commits 982ee86..ba6e48e, review clean) [handlers+auth; user decided keep 403; logger path-only fix; self-transfer/role-const/size-clamp fixes ba6e48e]
Task 14: complete (commits ba6e48e..8f330c5, review clean) [goose v3.27.2 Provider+session locker; serve/worker; graceful drain fix 8f330c5]
Task 15: complete (commit bb20f72, review clean) [rate limiter on auth routes; handler 400 test]
Task 16: complete (commit af9d7a5, review clean) [compose app-dev + Dockerfile copy internal; docker build + startup migrations verified]
Task 17: complete (verification, no fixups) [build/vet/lint(0)/unit/integration all green; govulncheck 0 on call path, 1 module advisory x/crypto/openpgp not called]

## Minor findings roll-up
- Task 1: mise `sqlc:generate` uses `sqlc@latest` (unpinned). Plan-mandated. Consider pinning a version before merge.
- Task 8: concurrency test single-direction; GetAccount errors discarded in assertion. Optional.
- Task 10: TLS policy set twice relying on last-wins ordering. Optional.
- Task 12: validator magic 400 + raw err leak — FIXED in Task 13 (http.StatusBadRequest + generic message).
- Task 13 (open, pending user decision):
  - IMPORTANT: getAccount returns 403 for not-owner vs 404 for missing = existence oracle. Spec explicitly said "403 if not owner". Security-better is uniform 404. -> ASK USER.
  - MINOR/SECURITY: verifyEmail secret code in `?code=` query may be logged by RequestLogger -> ensure logger excludes query string. -> ASK USER.
  - MINOR: self-transfer (from==to) not rejected; hardcoded "depositor" role literal; listAccounts size resets to 5 instead of clamp to 100; uuid.Parse errors ignored in transfer (safe due to validate tag).

## Task 13 RESOLVED
- getAccount: user chose KEEP 403 (spec). Accepted low-risk oracle.
- Logger: FIXED path-only, no query (ba6e48e).
- Minors: FIXED self-transfer reject, roleDepositor const, size clamp 100 (ba6e48e). uuid.Parse-ignored left as-is (safe via validate tag).

## Cross-task actions (must apply)
- Task 11: DONE — secret_code uses crypto/rand via util.SecureToken; FullName html-escaped in email body.
