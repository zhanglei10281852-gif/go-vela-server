# Worker 生命周期代次与旧凭据隔离

> **Workspace:** F:\swe\098905\project-03

---

**User**

平台管理员删除一个 worker 并用相同 hostname 注册替代节点后，旧节点尚未过期的 auth token 仍能命中新记录并更新或续期，之前签发的 register token 也可能再次创建同名节点。请把每次注册视为独立的 worker 生命周期代次：register token 只能建立其被授权的代次，返回的 auth token 只对该代次有效；删除一旦成功，旧代次的注册与认证凭据都不能再创建、更新、refresh 或影响后来同名的新记录。删除、重建与迟到的 PUT 或 refresh 并发交错时，旧请求必须被条件拒绝，不能覆盖新节点的地址、路由、运行中 build、状态或 check-in。创建数据库记录后若 auth token 签发失败，重试应能恢复或清除该未完成代次，而不是留下占用 hostname 却没有可用凭据的 worker。当前代次的正常注册、更新和续期、既有 token 过期时间、worker 列表与指标，以及明确配置的 server-worker 共享对称 token 模式保持兼容。

**Assistant**

toolName: view_folder

status: failed

f:\swe\098905\project-03

toolName: file_search

status: success

toolName: todo_write

status: success

Todos updated: 7 items

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\router\worker.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\router\middleware\worker.go

toolName: file_search

status: success

toolName: file_search

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\api\worker\create.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\api\worker\update.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\api\worker\refresh.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\api\worker\delete.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\router\middleware\perm\perm.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\router\middleware\worker\worker.go

toolName: file_search

status: success

toolName: file_search

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\internal\token\manager.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\internal\token\mint.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\internal\token\compose.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\router\middleware\claims\claims.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\internal\token\parse.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\worker.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\interface.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\create.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\update.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\delete.go

toolName: file_search

status: success

toolName: file_search

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\types\worker.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\api\types\worker.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\table.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\index.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\get_hostname.go

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\api\admin\worker.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\constants\token.go

Now let me look at existing tests and check how upstream structures things. Let me check the test files and go.mod.

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\go.mod

toolName: file_search

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\worker_test.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\create_test.go

Let me check how the upstream project addressed this, and look at related parts (build token minting using worker hostname, metrics, list).

toolName: web_search

status: success

toolName: search_by_regex

status: success

Let me examine the database setup/migration approach, the service interface, and metrics usage of workers.

toolName: view_files

status: failed

filePath: f:\swe\098905\project-03\database\service.go

toolName: search_by_regex

status: success

toolName: file_search

status: success

toolName: view_folder

status: success

f:\swe\098905\project-03\database

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\interface.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\database.go

Let me check how schema upgrades are handled for existing installs and look at remaining pieces.

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\api\metrics.go

Let me check the current upstream go-vela worker schema and registration implementation for alignment.

toolName: web_search

status: success

Upstream doesn't have this feature — I'll implement a self-contained design. Let me finish surveying the tests and routing that will be affected.

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\delete_test.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\update_test.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\table_test.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\get_test.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\get_hostname_test.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\testutils\mock_rows.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\types\worker_test.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\list_test.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\worker\opts.go

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\router\middleware\perm\perm_test.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\internal\token\mint_test.go

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

I now have a complete picture. Let me update the plan and start implementing.

toolName: todo_write

status: success

Todos updated: 7 items

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\constants\table.go

toolName: file_search

status: success

Now I'll implement. Starting with constants:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\constants\table.go

changes: undefined

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\constants\worker_status.go

toolName: Write

status: success

filePath: f:\swe\098905\project-03\constants\worker_register.go

changes: undefined

Now the token claims layer:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\internal\token\mint.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\internal\token\mint.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\internal\token\mint.go

changes: undefined

Now extend the API Worker type with an internal generation field:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\api\types\worker.go

changes: undefined

Now add the getter/setter for Generation (after GetBuildLimit):

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\api\types\worker.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\api\types\worker.go

changes: undefined

toolName: file_search

status: success

toolName: Write

status: success

filePath: f:\swe\098905\project-03\api\types\worker_register_token.go

changes: undefined

Now the DB types — add Generation to the worker DB type:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\types\worker.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\types\worker.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\types\worker.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\types\worker.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\types\worker.go

changes: undefined

Now the DB type for register tokens:

toolName: Write

status: success

filePath: f:\swe\098905\project-03\database\types\worker_register_token.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\types\worker_register_token.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\types\worker_register_token.go

changes: undefined

Now the database engine. First, table DDL:

toolName: Write

status: success

filePath: f:\swe\098905\project-03\database\worker\table.go

changes: undefined

toolName: Write

status: success

filePath: f:\swe\098905\project-03\database\worker\index.go

changes: undefined

Now wire the new table/index/migration into engine creation:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\worker.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\worker.go

changes: undefined

Now the register token persistence methods:

toolName: Write

status: success

filePath: f:\swe\098905\project-03\database\worker\register_token.go

changes: undefined

Now the transactional create, conditional update, and cascading delete:

toolName: Write

status: success

filePath: f:\swe\098905\project-03\database\worker\create.go

changes: undefined

toolName: Write

status: success

filePath: f:\swe\098905\project-03\database\worker\update.go

changes: undefined

toolName: Write

status: success

filePath: f:\swe\098905\project-03\database\worker\delete.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\interface.go

changes: undefined

Let me check how the integration test tracks interface methods:

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\integration_test.go

Now update the permission middleware so register tokens can't act as auth tokens:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\router\middleware\perm\perm.go

changes: undefined

Now the admin register token handler:

toolName: Write

status: success

filePath: f:\swe\098905\project-03\api\admin\worker.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\api\admin\worker.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\register_token.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\interface.go

changes: undefined

Now the registration handler with the full generation lifecycle:

toolName: Write

status: success

filePath: f:\swe\098905\project-03\api\worker\create.go

changes: undefined

Now the update handler with generation enforcement:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\api\worker\update.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\api\worker\update.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\api\worker\update.go

changes: undefined

Now the refresh handler:

toolName: Write

status: success

filePath: f:\swe\098905\project-03\api\worker\refresh.go

changes: undefined

Update the delete handler to map stale records:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\api\worker\delete.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\api\worker\delete.go

changes: undefined

Let me build to catch compile errors before fixing tests:

toolName: Shell

status: success

toolName: Shell

status: success

Build passes. Now run the affected tests:

toolName: Shell

status: success

Types and token tests pass. Update the postgres test helper expectations:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\worker_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\worker_test.go

changes: undefined

toolName: Shell

status: success

The gorm behavior is clear. Let me clean up the conditional update SQL — rely on Save's own primary-key condition:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\update.go

changes: undefined

Now update the sqlmock-based tests:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\create_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\update_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\delete_test.go

changes: undefined

toolName: Shell

status: success

Now extend the integration test to exercise the new methods:

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\integration_test.go

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\integration_test.go

changes: undefined

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

The resource test needs the new DDL expectations too:

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\database\resource_test.go

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\resource_test.go

changes: undefined

toolName: Shell

status: success

All database tests pass. Now the middleware and full suite:

toolName: Shell

status: success

Existing tests all pass. Now let me add lifecycle tests. First check how handler-level tests are structured in this repo:

toolName: file_search

status: success

toolName: file_search

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\api\schedule\create_test.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\router\middleware\worker\worker_test.go

I'll write comprehensive end-to-end handler tests. Check the claims context helper:

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\router\middleware\claims\context.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\api\types\token.go

Now write the database-layer generation lifecycle tests:

toolName: Write

status: success

filePath: f:\swe\098905\project-03\database\worker\generation_test.go

changes: undefined

toolName: Shell

status: success

gorm's `Save` upserts on sqlite, which is unsafe here. Switch to an explicit conditional `Updates`:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\update.go

changes: undefined

toolName: Shell

status: success

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\update.go

changes: undefined

toolName: Shell

status: success

gorm auto-appends the primary key. Rely on that and only add the generation condition:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\update.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\update.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\database\worker\update_test.go

changes: undefined

toolName: Shell

status: success

Now the end-to-end handler tests. Check user.Establish requirements:

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\router\middleware\user\user.go

Now write the end-to-end handler test:

toolName: Write

status: success

filePath: f:\swe\098905\project-03\api\worker\generation_test.go

changes: undefined

toolName: Shell

status: success

toolName: Shell

status: success

The subject mismatch check fires first (400). Fix the test to use an unregistered generation for the same hostname:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\api\worker\generation_test.go

changes: undefined

toolName: Shell

status: success

Add an admin handler test for register token issuance:

toolName: Write

status: success

filePath: f:\swe\098905\project-03\api\admin\worker_test.go

changes: undefined

toolName: todo_write

status: success

Todos updated: 7 items

toolName: Shell

status: success

Let me inspect the potentially-related mock/server failures and check which failures are pre-existing:

toolName: Shell

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\mock\server\worker_test.go

The mock test requires every Worker field in JSON. Let me check the fixture and expose `generation` additively:

toolName: search_by_regex

status: success

I'll keep generation out of the API JSON (so clients can't supply it) and make the mock test respect JSON tags:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\mock\server\worker_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-03\mock\server\worker_test.go

changes: undefined

toolName: Shell

status: success

toolName: Shell

status: success

Ruleset failure is pre-existing. Let me check all DeleteWorker callers and any stale-worker cleanup to ensure nothing else needs adjusting:

toolName: search_by_regex

status: success

Let me verify the admin route protection and run vet:

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\router\admin.go

toolName: Shell

status: success

Let me check DOCS.md for worker token documentation that may now be inaccurate:

toolName: search_by_regex

status: success

Let me run gofmt and the final targeted test sweep:

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

Let me verify no other routes relied on register tokens passing `MustWorkerAuthToken`:

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

All routes are coherent. Final review of the key files:

toolName: view_files

status: success

filePath: f:\swe\098905\project-03\api\worker\create.go

toolName: todo_write

status: success

Todos updated: 7 items

toolName: GetDiagnostics

status: success

完成。已把每次注册实现为独立的 worker 生命周期代次，旧代次凭据在删除/重建后无法再影响新记录。

## 核心设计

**1. 代次标识（generation）贯穿凭据与记录**
- JWT claims 新增 `worker_generation`：[mint.go](file:///f:/swe/098905/project-03/internal/token/mint.go)。register token 携带它"被授权建立"的代次，auth token 只携带它所属代次。
- workers 表新增 `generation` 列（[table.go](file:///f:/swe/098905/project-03/database/worker/table.go)，含旧库幂等升级语句），仅服务端内部使用（API 类型 `json:"-"`，客户端无法通过请求体伪造）。

**2. register token 服务端单次授权（新表 `worker_register_tokens`）**
- 管理员签发时同时入库（active、hostname、过期时间、UUID 代次）：[api/admin/worker.go](file:///f:/swe/098905/project-03/api/admin/worker.go)。
- 注册在**一个事务**中完成：插入 worker + 条件消费（`token_id + hostname + status=active + 未过期`，且 RowsAffected=1），否则回滚：[database/worker/create.go](file:///f:/swe/098905/project-03/database/worker/create.go)。用过的、过期的、hostname 不符的、已删除的 register token 都不能再建节点。
- auth token **先于** DB 记录签发；签发失败不会留下占 hostname 的空壳代次，重试即可恢复；管理员签发 JWT 失败时还会回滚登记记录。

**3. 删除即吊销、迟到请求条件拒绝**
- 删除在事务内同时删除 worker 与该 hostname 的全部 register token：[delete.go](file:///f:/swe/098905/project-03/database/worker/delete.go)。
- PUT/refresh 处理器先比对 claims 代次与当前记录（不符 401）：[update.go](file:///f:/swe/098905/project-03/api/worker/update.go)、[refresh.go](file:///f:/swe/098905/project-03/api/worker/refresh.go)；DB 层更新带 `WHERE id AND generation` 条件且**只 UPDATE 不 upsert**，0 行返回 `ErrStaleWorker`（409）——删除/重建窗口内的迟到 PUT/refresh 无法覆盖新节点的地址、路由、running builds、状态或 check-in。
- `MustWorkerAuthToken` 不再接受 register token（[perm.go](file:///f:/swe/098905/project-03/router/middleware/perm/perm.go)），它只能用于建立代次。

**4. 兼容性**
- 对称 server-worker token 模式完全不绑定代次，注册/更新/续期行为不变；
- 旧库的 generation 为空行 + 旧的空代次 auth token 仍可正常更新/续期直到自然过期，但一旦删除重建，新行绑定代次，旧 token 立即失效；
- worker 列表、指标、token 时长与过期校验均未改动。

## 验证

新增测试覆盖：注册消费/重放、过期/未知 token 回滚不占 hostname、删除级联吊销、旧代次迟到写被拒且新行数据不变、重建后新旧凭据 401/404/409、失败重试恢复、对称模式与 legacy token——端到端见 [api/worker/generation_test.go](file:///f:/swe/098905/project-03/api/worker/generation_test.go)，数据库层见 [database/worker/generation_test.go](file:///f:/swe/098905/project-03/database/worker/generation_test.go)。全部相关包 `go test` 通过（`TestPipeline_Ruleset_Match` 与 scm/github 网络用例在改动前即失败，与本次无关）。
