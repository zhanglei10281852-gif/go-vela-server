# 日志清理所有权与中断恢复

> **Workspace:** F:\swe\098905\project-01

---

**User**

平台管理员发起大批量日志清理时，如果客户端中途断开、server 实例重启，或某个分区在删除了部分批次后报错，当前清理锁可能无法由原请求释放，后续操作会一直被判定为已有任务；也可能在锁实际归属不明时继续删除或回退到另一条路径，导致统计遗漏、重复处理和无法判断清理是否完成。请让每次清理拥有可持久识别的执行代次和有期限的所有权，PostgreSQL 与 SQLite 都只能由当前所有者执行删除、分区回退和 vacuum；请求取消或实例退出后，其他实例能够安全接管尚未完成的同一清理，而迟到的旧实例不得继续修改数据或释放新所有者的锁。跨分区或批次发生部分失败时，已完成进度、受影响分区和删除数量要可恢复，重试从确定位置继续并给出准确结果，不能遗漏、重复计数或永久占用清理入口。正常串行清理、非分区模式、无匹配日志、参数校验、默认批大小以及 vacuum 关闭时的既有行为保持兼容。

**Assistant**

toolName: view_folder

status: failed

f:\swe\098905\project-01

toolName: Shell

status: success

toolName: todo_write

status: success

Todos updated: 8 items

toolName: search_by_regex

status: success

toolName: Shell

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\api\admin\log.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\api\admin\clean.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\log\interface.go

toolName: file_search

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\log\log.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\log\opts.go

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\interface.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\api\types\log.go

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\log\log_test.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\log\table.go

toolName: Shell

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\api\admin\log_test.go

Let me look at a few more supporting files before designing the solution.

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\database.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\cmd\vela-server\cleanup.go

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\integration_test.go

Now let me check the SQLite driver version and a couple more details.

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\log\delete.go

I have a complete picture now. Let me verify a couple of final details before implementing.

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: todo_write

status: success

Todos updated: 6 items

Now I'll implement the new lease-based, generation-identified cleanup engine:

toolName: Write

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

changes: undefined

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

changes: undefined

toolName: Shell

status: success

The build passes. Now let me extract two SQL constants so tests can reference exact query text, then write the comprehensive test suite:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

changes: undefined

Now the test suite:

toolName: Write

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

changes: undefined

Let me clean up the test helpers — I left an invalid shim and used non-existent `.Rows()` on gorm Exec results:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

changes: undefined

Now fix the fenced-owner test's seed and log inserts plus `errorsIs`:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

changes: undefined

toolName: Shell

status: success

toolName: Shell

status: success

The postgres mock exposed a design issue: after INSERT/UPDATE I re-read the lease row, but mocks can't echo the generated owner ID. Cleaner design: use `INSERT ... RETURNING` / `UPDATE ... RETURNING` (supported by both PostgreSQL and modern SQLite) to get the authoritative lease directly. Let me refactor:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

changes: undefined

toolName: Shell

status: success

All new tests pass. Now run the full affected package suites to check for regressions:

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

My two files are gofmt-clean (the listed files are pre-existing and untouched). Let me run the entire test suite to verify no regressions:

toolName: Shell

status: success

The only failures are in `scm/github` — unrelated to this change. Let me confirm they're pre-existing/environmental:

toolName: Shell

status: success

The `scm/github` failures are pre-existing Windows CRLF testdata issues, unrelated to this change. Now let me refine two subtleties I spotted on review:

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

changes: undefined

Now refine vacuum bookkeeping so a failed vacuum is retried by a takeover rather than marked done:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

changes: undefined

Now let me add one more end-to-end test: client disconnect mid-cleanup → immediate takeover by a second request without waiting for TTL:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

changes: undefined

That edit mangled the file — let me view and fix it, adding the new test properly:

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean_test.go

changes: undefined

toolName: Shell

status: success

All scenarios pass. Let me run with the race detector on the affected packages, plus full vet:

toolName: Shell

status: success

Race-clean. Let me do a final review of the complete implementation:

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

changes: undefined

toolName: search_by_regex

status: success

toolName: Shell

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

The logic is coherent. One small accuracy polish: on takeover, the cleanup continues with the *persisted* batch size, but the handler derives `batches_processed` from the new request's `batch_size`. Let me return the effective batch size so that stat stays accurate:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\database\log\clean.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-01\api\admin\log.go

changes: undefined

toolName: Shell

status: success

toolName: todo_write

status: success

Todos updated: 6 items

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

The diff is minimal and intentional. Final full verification across related packages:

toolName: Shell

status: success

完成。已将日志清理改造为**持久代次（generation）+ 有期限所有权（lease/epoch fencing）+ 可恢复进度**模型，PostgreSQL 与 SQLite 行为一致。

## 原有缺陷

- PostgreSQL 的 `pg_try_advisory_lock` 是会话级锁，经 gorm 连接池获取/释放可能落在不同连接上；SQLite 的 `log_cleanup_coordination` 行提交后永久存在——进程崩溃即**永久锁死清理入口**。
- 锁没有归属与期限：客户端断开、实例重启、分区删了一半报错后，旧实例可能继续删或静默回退到传统路径，计数归零、受影响分区丢失。
- 无持久进度：重试从头开始，删除数量遗漏/无法核对。

## 改动文件

- [database/log/clean.go](file:///f:/swe/098905/project-01/database/log/clean.go)（核心重写）
- [database/log/clean_test.go](file:///f:/swe/098905/project-01/database/log/clean_test.go)（9 个场景）
- [api/admin/log.go](file:///f:/swe/098905/project-01/api/admin/log.go#L164-L177)（仅批次统计改用实际生效批大小）

## 关键设计

1. **持久代次 + 有期限所有权**：新增单例表 `log_cleanup_leases`（PG/SQLite 通用 DDL），记录 `generation`、`epoch`（fencing token）、`owner_id`、`leased_until`(30s TTL)、`status` 及全部进度。
   - 无行 → 新代次；已完成 → 新 generation；`running` 且租约有效 → 拒绝（沿用原报错 "another cleanup operation is already in progress"）；**租约过期 → 同一代次接管，epoch+1**（旧实例被 fence）。
   - 活所有者每批次随事务续租 + 10s 心跳；心跳发现 epoch 失效立即取消运行上下文。
2. **所有写操作只认当前所有者**：删除/分区回退/vacuum/释放/完结的 SQL 全部带 `owner_id=? AND epoch=? AND status='running'`。每批在**同一事务**内执行 `DELETE ... RETURNING id`（id 键集游标，确定续做点）+ 进度 UPDATE；条件不命中则整批回滚（删除也撤销），迟到旧实例无法改数据，也**无法释放新所有者的锁**。
3. **取消/退出可安全接管**：请求取消或出错时用脱离请求的上下文把 `leased_until` 立即置为过去（崩溃则等 TTL），其他实例马上接管同一 generation 继续；验证了旧实例 fencing 后删除回滚、释放/完结均空操作。
4. **部分失败可恢复、计数精确**：删除数在数据库内累加、全局/分区游标、分区快照 JSON、`partition_index`、受影响分区列表随批次提交；分区→传统回退变为**经所有者校验并持久化的模式切换**，计数与受影响分区保留（测试：50 分区 + 3 回退 = 53，不重不漏）；vacuum 前有所有者检查点，失败保持告警且 `vacuum_done` 不置位，接管会补做。
5. **兼容既有行为**：串行互斥、非分区模式、无匹配日志、参数校验、默认 1000/上限 10000、vacuum 默认关闭、100ms 批间隔、响应结构与状态码均不变。

## 验证

`go build ./...`、`go vet`、`go test ./database/... ./api/... ./cmd/... ./router/...` 全部通过；`database/log` 与 `api/admin` 在 `-race` 下通过。新增场景含：正常串行（PG mock 带 vacuum、SQLite 真实库）、空库、取消、活跃租约拒绝、**从游标 3 接管续做（3+7=10，新日志保留，epoch=2）**、**fencing 旧实例删除回滚且不能放锁**、**运行中取消后第二请求立即接管且总数精确为 200**、双分区清理、分区失败回退累计统计。（`scm/github` 的失败是仓库既有的 Windows CRLF 测试数据问题，与本次改动无关。）
