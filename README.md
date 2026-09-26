# 旋风分离核算服务 (Cyclone Sizing Service)

基于 **Stairmand 标准几何比例**与切割粒径公式的常驻 HTTP 核算服务，用 Go 1.22 + Gin 实现。

## 功能

- **筒体几何**：按 Stairmand 高效旋风分离器比例，由筒径 D 确定入口宽高、排气管径、筒身/锥段长度、排尘口径及有效旋转圈数。
- **切割粒径 d50**：在 Stokes 区间按 Stairmand 切割公式计算 50% 捕集粒径，同时给出 d50 处颗粒雷诺数并判别流态；超出 Stokes 区时在 `warnings` 中给出警告，而不是假装公式仍然准确。
- **单点分级效率**：对任意指定粒径按经验分级曲线（Lapple 切割曲线 η = 1/(1+(d50/d)²)）计算捕集效率与穿透率。曲线随粒径严格单调上升，η(d50)=0.5，不会饱和为统一的 100%。
- **粒径分布积分**：给一条带权重的粒径分布，按 η_overall = Σ wᵢ·η(dᵢ) / Σ wᵢ 加权积分出总体分级效率（权重自动归一化，非单位和只警告不报错）。

## 模块划分（比例常数与切割公式只在一处定义）

```
cyclone/
  types.go       基本参数与结构化错误 APIError
  geometry.go    Stairmand 几何比例（所有比例常数的唯一定义处）
  cutpoint.go    d50 切割公式（公式唯一定义处）、雷诺数与流态判别
  efficiency.go  单点分级效率 + 粒径分布加权积分（d50 一律走 cutpoint 模块）
httpapi/
  handler.go     四个核算能力的处理器
  router.go      路由装配、错误码到 HTTP 状态码的映射
main.go          启动入口
```

效率模块不维护任何几何系数：它通过 `EvaluateCutPoint` 拿 d50，d50 内部再从 `geometry.go` 取比例系数 b/D 与 N，因此几何与效率之间不会出现两份不一致的比例。

## 构建与运行

```bash
# 直接运行
go run .

# 构建镜像（受限时可 --build-arg GOPROXY=https://goproxy.cn,direct）
docker build -t cyclone-service .
docker run --rm -p 8080:8080 cyclone-service
```

服务监听 `:8080`（可用 `PORT` 环境变量改）。

## API

所有参数 SI 单位：筒径 m、入口风速 m/s、密度 kg/m³、粘度 Pa·s、粒径 m。

### 1. 筒体几何

```bash
curl -s -X POST localhost:8080/api/v1/cyclone/geometry \
  -H 'Content-Type: application/json' \
  -d '{"cylinder_diameter":1.0}'
```

### 2. 切割粒径与雷诺数

```bash
curl -s -X POST localhost:8080/api/v1/cyclone/cut-point \
  -H 'Content-Type: application/json' \
  -d '{"cylinder_diameter":1.0,"inlet_velocity":20,
       "gas_density":1.2,"solid_density":2000,"gas_viscosity":1.8e-5}'
```

返回（节选）：`cut_diameter_d50_m` / `cut_diameter_d50_um` / `particle_reynolds` / `regime` / `stokes_assumption_valid` / `warnings` / `geometry`。

### 3. 单点分级效率

```bash
curl -s -X POST localhost:8080/api/v1/cyclone/grade-efficiency \
  -H 'Content-Type: application/json' \
  -d '{"cylinder_diameter":1.0,"inlet_velocity":20,
       "gas_density":1.2,"solid_density":2000,"gas_viscosity":1.8e-5,
       "particle_diameter_m":8.03e-6}'
```

### 4. 粒径分布积分

```bash
curl -s -X POST localhost:8080/api/v1/cyclone/distribution-efficiency \
  -H 'Content-Type: application/json' \
  -d '{"cylinder_diameter":1.0,"inlet_velocity":20,
       "gas_density":1.2,"solid_density":2000,"gas_viscosity":1.8e-5,
       "bins":[{"diameter_m":2e-6,"weight":0.3},
               {"diameter_m":8e-6,"weight":0.7}]}'
```

另：`GET /healthz` 存活探针。

## 错误处理

非法输入返回结构化错误，语义非法为 `422`，JSON 解析失败为 `400`：

```json
{"error":{"code":"INVALID_DENSITY","field":"solid_density",
          "message":"solid density must be greater than gas density"}}
```

拒绝条件：筒径/入口风速 ≤ 0；固体密度 ≤ 气体密度；粘度 ≤ 0；粒径 ≤ 0；分布为空、权重为负或总和 ≤ 0；以及 NaN/Inf。

## Stairmand 几何比例（相对筒径 D）

| 部位 | 符号 | 比例 |
|---|---|---|
| 入口宽 | a/D | 0.20 |
| 入口高 | b/D | 0.50 |
| 排气管（溢流管）径 | De/D | 0.50 |
| 排气管插入长 | S/D | 0.50 |
| 圆筒段长 | h/D | 1.50 |
| 总高 | H/D | 4.00 |
| 排尘口径 | B/D | 0.375 |
| 有效旋转圈数 | N | 5 |

## 测试

```bash
go test -race ./...
```

被自动化测试钉住的关系：

- 仅入口速度翻倍 → d50 ≈ /√2（且明确排除 0.5 的线性反比）；
- 仅气固密度差翻倍 → d50 ≈ /√2；
- 仅筒径翻倍（比例不变、风速不变）→ d50 变粗（≈√2 倍）；
- 气体粘度翻倍 → d50 变粗（≈√2 倍）；
- 分级效率在对数粒径序列上严格单调、不出现 100% 饱和；
- 同一条分布、d50 调大后总体分级效率严格下降；
- Re 超出 Stokes 区时必须带警告；
- 各类非法输入（零/负筒径风速、密度倒挂、非正粘度、坏粒径/分布）逐一拒绝；
- HTTP 层对 400/422/404 与成功响应做端到端校验。
