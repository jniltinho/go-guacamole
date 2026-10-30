# Codex review of the implementation (task 8.2)

Read-only review of `78bf666..HEAD`: 8 high, 7 medium, 1 low. Each finding and what was done.

| # | Finding | Outcome |
|---|---|---|
| 1 | Local removal could unlink after a hold was accepted (claim not renewed while hashing) | Fixed: the claim is renewed (fenced) right before unlinking; a lost claim stops the removal |
| 2 | Purge had no fenced per-set claim; a second purge could recover sets in flight | Fixed: `BeginPurge` takes the set's claim with a token, `RecoverPurging` only recovers expired claims, `EndPurge` is fenced, and the claim is renewed before unlinking |
| 3 | Purge decided from a snapshot taken before `BeginPurge` | Fixed: the row is read again after the set is claimed |
| 4 | Objects of a half-failed upload were not tracked | Fixed: `PlanUpload` records target and keys before writing; the purge deletes planned keys of failed/owed sets; rollback of 0008 is refused while any set has a target and is not deleted |
| 5 | A stale upload's cleanup deleted the whole directory (sibling set included) | Fixed: only the keys that run wrote (`DeleteSet`) |
| 6 | The kept version is not necessarily the verified one | Mitigated: the reconciliation prunes older versions only after verifying the current objects against the manifest (SHA-256); on a mismatch every version is kept and the set is uploaded again (or logged when the local copy is gone). Version IDs are not stored: rclone's S3 versions view does not expose them for reads |
| 7 | `DeleteSet` matched by file name, so `<dir>/backup/<uuid>` was deleted too | Fixed: full keys; test with a same-named key in a subdirectory |
| 8 | GCS / OCI native versioned buckets keep old versions after deletion | Answered: rclone exposes no version listing for these backends; documented (operations.md) and the Storage panel now warns to keep versioning off for these providers |
| 9 | Claim and lease renewal checked validity separately from the write; `CompletePeriod` could record after losing the lease | Fixed: single conditional `UPDATE`/`DELETE` with `RowsAffected` checks; `CompletePeriod` deletes the lease first and records nothing when it was lost |
| 10 | Sets of sessions ended by another instance (dead owner) were never discovered | Fixed: the reconciliation discovers unchecked file sets before marking uploads owed |
| 11 | Sanitizer missed `X-Amz-Security-Token:` headers and XML/JSON signing echoes | Fixed: header, `<SignatureProvided>`/`<StringToSign>`/`<CanonicalRequest>`/token tags and OAuth token fields; tests |
| 12 | Redirects not disabled in backend clients | Answered: every connection goes through the in-process egress proxy, which checks the resolved IP at dial time, so a redirect to a forbidden address is refused like any other request |
| 13 | Rotated credentials stayed cached on other replicas | Fixed: the target is read on every use and the connection is reused only while it is unchanged |
| 14 | Rollback stayed blocked after a set was fully deleted | Fixed: deleted sets (local removed, bucket purged) no longer block it; test |
| 15 | Long hostnames overflow `VARCHAR(64)` instance ids | Fixed: hostname capped at 55 characters |
| 16 | Access key ID is returned by the API | Answered: kept visible on purpose (it identifies the key, like cloud consoles show it); the secret key and service account JSON stay write-only |
