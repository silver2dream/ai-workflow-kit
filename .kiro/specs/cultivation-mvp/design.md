# 修仙之路 App - MVP 設計文件

## 概述

本設計文件定義《修仙之路》App 的技術架構、資料模型、API 設計和系統組件。系統採用 Unity (C#) 前端 + Nakama (Go) 後端的架構，使用 AI 服務生成個性化的修仙故事內容。

### 核心技術棧

**前端 (Unity)**
- R3：反應式程式設計框架
- UniTask：非同步任務處理
- MessagePipe：事件總線與依賴注入
- UI Toolkit：UI 系統
- Unity Localization：多國語言支援
- Firebase SDK：Analytics 與 Remote Config

**後端 (Nakama)**
- Go Runtime：伺服器邏輯
- MongoDB：玩家遊戲資料儲存
- PostgreSQL：金流與交易資料儲存
- Redis：快取與 Session 管理
- JWT：身份驗證

**AI 服務**
- OpenAI GPT-4 / Claude：故事生成
- Stable Diffusion / Flux：心印閃卡圖像生成

## 架構設計

### 系統架構圖

```
┌─────────────────────────────────────────────────────────────┐
│                      Unity Client                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   UI Layer   │  │ Domain Layer │  │Infrastructure│     │
│  │  (Views/VM)  │◄─┤  (Business)  │◄─┤   (Network)  │     │
│  └──────────────┘  └──────────────┘  └──────┬───────┘     │
│         ▲                                     │              │
│         │ R3 Events                          │ HTTPS/JWT   │
│         ▼                                     ▼              │
│  ┌──────────────┐                   ┌──────────────┐       │
│  │  EventBus    │                   │  Firebase    │       │
│  │ (MessagePipe)│                   │  Analytics   │       │
│  └──────────────┘                   └──────────────┘       │
└─────────────────────────────────────────┬───────────────────┘
                                          │
                                          │ RPC Calls
                                          ▼
┌─────────────────────────────────────────────────────────────┐
│                    Nakama Server (Go)                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Destiny    │  │   Chapter    │  │ Breakthrough │     │
│  │   Module     │  │   Module     │  │   Module     │     │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘     │
│         │                  │                  │              │
│         └──────────────────┼──────────────────┘              │
│                            ▼                                 │
│                   ┌──────────────┐                          │
│                   │  AI Service  │                          │
│                   │   Gateway    │                          │
│                   └──────┬───────┘                          │
│                          │                                   │
│         ┌────────────────┼────────────────┐                │
│         ▼                ▼                ▼                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐│
│  │ MongoDB  │  │PostgreSQL│  │  Redis   │  │ AI APIs  ││
│  │ (Game)   │  │(Payment) │  │ (Cache)  │  │(GPT/SD)  ││
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘│
└─────────────────────────────────────────────────────────────┘
```

### 前端架構

```
Assets/Scripts/
├── Core/
│   └── GameEntry.cs                    # 遊戲入口點
├── Domain/
│   ├── Destiny/
│   │   ├── DestinyModel.cs            # 命格資料模型
│   │   ├── DestinyService.cs          # 命格業務邏輯
│   │   └── DestinyEvents.cs           # 命格相關事件
│   ├── Cultivation/
│   │   ├── CultivationModel.cs        # 修煉資料模型
│   │   ├── CultivationService.cs      # 修煉業務邏輯
│   │   └── CultivationEvents.cs       # 修煉相關事件
│   ├── Chapter/
│   │   ├── ChapterModel.cs            # 章節資料模型
│   │   ├── ChapterService.cs          # 章節業務邏輯
│   │   └── ChapterEvents.cs           # 章節相關事件
│   └── Config/
│       ├── Models/
│       │   ├── GlobalConfig.cs        # 全域配置資料類
│       │   ├── RealmConfig.cs         # 境界配置資料類
│       │   └── SpiritualRootConfig.cs # 靈根配置資料類
│       └── ConfigService.cs           # 配置查詢服務（業務邏輯層）
├── Infrastructure/
│   ├── Network/
│   │   ├── NetworkManager.cs          # Nakama 網路管理
│   │   └── RpcPayloads.cs             # RPC 請求/回應結構
│   ├── Analytics/
│   │   └── AnalyticsManager.cs        # Firebase Analytics
│   ├── Storage/
│   │   └── LocalStorageManager.cs     # 本地資料儲存
│   ├── Config/
│   │   └── ConfigLoader.cs            # 配置載入器（從 Resources 載入 JSON）
│   └── Localization/
│       └── LocalizationManager.cs     # 多國語言管理
├── Managers/
│   └── UIManager.cs                    # UI 管理器
└── UI/
    ├── Views/
    │   ├── DestinyCreationView.cs     # 命格生成介面
    │   ├── ChapterReadingView.cs      # 章節閱讀介面
    │   ├── BreakthroughView.cs        # 突破介面
    │   └── HeartSealCardView.cs       # 心印閃卡介面
    └── ViewModels/
        ├── DestinyCreationViewModel.cs
        ├── ChapterReadingViewModel.cs
        ├── BreakthroughViewModel.cs
        └── HeartSealCardViewModel.cs
```


### 後端架構

> NOTE (implementation): 後端程式碼結構以 `.claude/rules/backend-nakama-architecture-and-patterns.md` 為準：
> - 不要新增或依賴 `*_rpc.go`
> - RPC entrypoints 一律放在 `<module>_service.go`（`Rpc*` methods），並與 usecases 分區
> - 從 ctx 取 user/session 資訊要安全（不可 panic）

```
backend/immortal-backend/
├── README-zh-TW.md
├── README.md
├── api
│   └── README.md
├── cmd
│   ├── genregister
│   │   └── main.go
│   └── nakama
│       └── main.go
├── configs
│   └── nakama-local.yml
├── coverage.html
├── db
│   └── migrations
│       └── postgres
│           ├── 000001_init_schema.down.sql
│           └── 000001_init_schema.up.sql
├── deployments
│   └── docker
│       ├── DebugDockerfile
│       ├── Dockerfile
│       ├── debug-docker-compose.yml
│       ├── docker-compose.yml
│       └── prometheus.yml
├── examples
│   ├── module
│   │   └── example
│   │       ├── example.go
│   │       └── example_module.go
│   └── redis
│       └── main.go
├── go.mod
├── go.sum
├── internal
│   ├── moduleimports
│   │   ├── register.go
│   │   └── register_gen.go
│   └── modules
│       ├── admin
│       │   ├── admin_module.go
│       │   └── admin_service.go
│       ├── chapter
│       │   ├── chapter_cache_redis.go
│       │   ├── chapter_module.go
│       │   ├── chapter_repository.go
│       │   ├── chapter_repository_mongo.go
│       │   ├── chapter_service.go
│       │   ├── generation_worker.go
│       │   ├── models.go
│       │   ├── task_runner.go
│       │   ├── types.go
│       │   ├── wire.go
│       │   └── wire_gen.go
│       ├── config
│       │   ├── INTEGRATION_GUIDE.md
│       │   ├── README.md
│       │   ├── config_audit.go
│       │   ├── config_module.go
│       │   ├── config_service.go
│       │   └── model
│       │       └── config.go
│       ├── destiny
│       │   ├── destiny_cache_redis.go
│       │   ├── destiny_module.go
│       │   ├── destiny_repository.go
│       │   ├── destiny_repository_mongo.go
│       │   ├── destiny_service.go
│       │   ├── destiny_service_test.go
│       │   ├── destiny_task_runner.go
│       │   ├── destiny_worker.go
│       │   └── models.go
│       ├── health
│       │   ├── health_module.go
│       │   ├── health_service.go
│       │   ├── health_service_test.go
│       │   └── models.go
│       └── qi
│           ├── METRICS_EXAMPLE.md
│           ├── models.go
│           ├── qi_cache_redis.go
│           ├── qi_module.go
│           ├── qi_repository.go
│           ├── qi_repository_mongo.go
│           ├── qi_repository_postgres.go
│           ├── qi_service.go
│           ├── qi_service_test.go
│           └── qi_verifier.go
├── migrations
│   └── postgres
│       ├── 001_create_qi_transactions.down.sql
│       ├── 001_create_qi_transactions.up.sql
│       ├── 002_create_outbox.down.sql
│       ├── 002_create_outbox.up.sql
│       └── README.md
├── mocks
│   ├── mock_AdVerifier.go
│   ├── mock_ChapterCache.go
│   ├── mock_ChapterCultivationRepository.go
│   ├── mock_ChapterRepository.go
│   ├── mock_ChapterTaskRepository.go
│   ├── mock_ChapterTaskRunner.go
│   ├── mock_DailyRestoreRepository.go
│   ├── mock_DestinyCache.go
│   ├── mock_DestinyTaskRunner.go
│   ├── mock_IDestinyRepository.go
│   ├── mock_PlayerDestinyRepository.go
│   ├── mock_PurchaseVerifier.go
│   ├── mock_QiCache.go
│   ├── mock_QiCultivationRepository.go
│   └── mock_QiTransactionRepository.go
├── pkg
│   ├── README.md
│   ├── ai
│   │   ├── providers
│   │   │   └── langchaingo_adapter.go
│   │   ├── registry.go
│   │   ├── service.go
│   │   └── types.go
│   ├── db
│   │   ├── mongo
│   │   │   └── client.go
│   │   ├── postgres
│   │   │   └── client.go
│   │   └── redis
│   │       └── client.go
│   ├── infra
│   │   └── database.go
│   ├── modules
│   │   ├── middleware
│   │   │   ├── errors_collector.go
│   │   │   └── middleware.go
│   │   └── modules.go
│   └── observability
│       ├── logger.go
│       ├── logger_example.go
│       ├── logger_test.go
│       ├── metrics.go
│       ├── metrics_helpers.go
│       ├── metrics_test.go
│       └── prometheus.go
├── scripts
│   ├── verify-nakama-versions.bat
│   └── verify-nakama-versions.sh
└── test
    ├── README.md
    ├── chapter_service_test.go
    ├── config_service_test.go
    ├── health_service_test.go
    ├── integration_chapter_qi_test.go
    ├── integration_db_test.go
    ├── logger_test.go
    ├── mocks.go
    └── qi_service_test.go
```

## 資料模型

### 資料儲存策略

| 資料類型 | 儲存位置 | 原因 |
|---------|---------|------|
| 玩家遊戲資料 | MongoDB | 高頻讀寫、靈活 schema |
| 金流交易資料 | PostgreSQL | ACID 保證、審計需求 |
| 即時狀態 | Redis | 快速存取、TTL 管理 |
| 遊戲配置 | JSON 檔案 | 企劃可編輯、版本控制 |

### MongoDB Collections

#### players (玩家基本資料)

```json
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "name": "玩家姓名",
  "gender": "male|female|other",
  "birth_date": "1990-01-01",
  "created_at": "2025-11-12T00:00:00Z",
  "last_login": "2025-11-12T10:00:00Z",
  "language": "zh-TW"
}
```

**索引**
- `user_id`: unique
- `last_login`: 用於活躍度查詢

#### destinies (命格資料)

```json
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "five_elements": {
    "metal": 20,
    "wood": 15,
    "water": 25,
    "fire": 10,
    "earth": 30
  },
  "spiritual_root": "earth",
  "destiny_description": "AI 生成的命格描述",
  "origin_story": "AI 生成的出身背景故事",
  "created_at": "2025-11-12T00:00:00Z"
}
```

**索引**
- `user_id`: unique

#### cultivations (修煉資料)

```json
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "current_qi": 35,
  "max_qi": 50,
  "current_realm": "qi_refining_early",
  "realm_progress": 45.5,
  "breakthrough_attempts": 0,
  "last_qi_reset": "2025-11-12T00:00:00Z",
  "updated_at": "2025-11-12T10:30:00Z"
}
```

**索引**
- `user_id`: unique
- `current_realm`: 用於統計分析

#### chapters (章節資料)

```json
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "chapter_id": "uuid",
  "chapter_number": 1,
  "title": "初入修仙界",
  "content": "AI 生成的章節內容...",
  "chapter_type": "normal|breakthrough|enlightenment",
  "realm_at_creation": "qi_refining_early",
  "created_at": "2025-11-12T10:00:00Z",
  "read_at": "2025-11-12T10:05:00Z"
}
```

**索引**
- `user_id` + `chapter_number`: compound unique
- `user_id` + `created_at`: 用於查詢歷史

### PostgreSQL Tables

#### subscriptions (訂閱資料)

```sql
CREATE TABLE subscriptions (
  id SERIAL PRIMARY KEY,
  user_id VARCHAR(128) NOT NULL,
  is_vip BOOLEAN DEFAULT FALSE,
  subscription_type VARCHAR(50),
  start_date TIMESTAMP NOT NULL,
  end_date TIMESTAMP NOT NULL,
  platform VARCHAR(20) NOT NULL,
  transaction_id VARCHAR(255) UNIQUE NOT NULL,
  auto_renew BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_subscriptions_user_id ON subscriptions(user_id);
CREATE INDEX idx_subscriptions_end_date ON subscriptions(end_date);
```

#### transactions (交易記錄)

```sql
CREATE TABLE transactions (
  id SERIAL PRIMARY KEY,
  user_id VARCHAR(128) NOT NULL,
  transaction_type VARCHAR(50) NOT NULL,
  amount DECIMAL(10, 2) NOT NULL,
  currency VARCHAR(10) NOT NULL,
  platform VARCHAR(20) NOT NULL,
  platform_transaction_id VARCHAR(255) UNIQUE NOT NULL,
  status VARCHAR(20) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_transactions_user_id ON transactions(user_id);
CREATE INDEX idx_transactions_created_at ON transactions(created_at);
```

### Redis Keys

| Key Pattern | Value Type | TTL | 用途 |
|------------|-----------|-----|------|
| `cultivation:{user_id}` | Hash | 1 hour | 快取修煉狀態 |
| `chapter:{user_id}:{chapter_num}` | String | 24 hours | 快取章節內容 |
| `vip:{user_id}` | String | 1 hour | 快取 VIP 狀態 |
| `ai_cache:{prompt_hash}` | String | 永久 | AI 生成結果快取 |


### 遊戲配置表 (Game Configurations)

所有配置檔案儲存於 `Resources/Configs/` 目錄，使用 JSON 格式。

#### GlobalConfig.json (全域配置)

```json
{
  "version": "1.0.0",
  "qi_settings": {
    "daily_qi_restore": 50,
    "max_qi": 50,
    "qi_per_chapter": 10,
    "ad_qi_restore": 10
  },
  "realm_settings": {
    "progress_per_chapter": 10,
    "max_progress": 100
  },
  "vip_settings": {
    "monthly_price_usd": 4.99,
    "monthly_price_twd": 150
  },
  "ai_settings": {
    "default_model": "gpt-4-mini",
    "max_tokens": 800,
    "temperature": 0.8
  }
}
```

#### RealmConfig.json (境界配置)

```json
{
  "realms": [
    {
      "id": "qi_refining_early",
      "order": 1,
      "name_key": "realm.qi_refining_early.name",
      "description_key": "realm.qi_refining_early.description",
      "required_progress": 100,
      "breakthrough_base_rate": 0.75,
      "next_realm": "qi_refining_mid",
      "element_bonus": {
        "metal": 0.05,
        "wood": 0.05,
        "water": 0.05,
        "fire": 0.05,
        "earth": 0.05
      }
    },
    {
      "id": "qi_refining_mid",
      "order": 2,
      "name_key": "realm.qi_refining_mid.name",
      "description_key": "realm.qi_refining_mid.description",
      "required_progress": 100,
      "breakthrough_base_rate": 0.70,
      "next_realm": "qi_refining_late",
      "element_bonus": {
        "metal": 0.08,
        "wood": 0.08,
        "water": 0.08,
        "fire": 0.08,
        "earth": 0.08
      }
    },
    {
      "id": "qi_refining_late",
      "order": 3,
      "name_key": "realm.qi_refining_late.name",
      "description_key": "realm.qi_refining_late.description",
      "required_progress": 100,
      "breakthrough_base_rate": 0.65,
      "next_realm": "foundation_early",
      "element_bonus": {
        "metal": 0.10,
        "wood": 0.10,
        "water": 0.10,
        "fire": 0.10,
        "earth": 0.10
      }
    }
  ]
}
```

#### SpiritualRootConfig.json (靈根配置)

```json
{
  "spiritual_roots": [
    {
      "id": "metal",
      "name_key": "spiritual_root.metal.name",
      "description_key": "spiritual_root.metal.description",
      "breakthrough_modifier": 0.05,
      "compatible_elements": ["earth", "water"],
      "incompatible_elements": ["fire", "wood"]
    },
    {
      "id": "wood",
      "name_key": "spiritual_root.wood.name",
      "description_key": "spiritual_root.wood.description",
      "breakthrough_modifier": 0.03,
      "compatible_elements": ["water", "fire"],
      "incompatible_elements": ["metal"]
    },
    {
      "id": "water",
      "name_key": "spiritual_root.water.name",
      "description_key": "spiritual_root.water.description",
      "breakthrough_modifier": 0.08,
      "compatible_elements": ["metal", "wood"],
      "incompatible_elements": ["earth"]
    },
    {
      "id": "fire",
      "name_key": "spiritual_root.fire.name",
      "description_key": "spiritual_root.fire.description",
      "breakthrough_modifier": 0.02,
      "compatible_elements": ["wood", "earth"],
      "incompatible_elements": ["water"]
    },
    {
      "id": "earth",
      "name_key": "spiritual_root.earth.name",
      "description_key": "spiritual_root.earth.description",
      "breakthrough_modifier": 0.06,
      "compatible_elements": ["fire", "metal"],
      "incompatible_elements": ["wood"]
    }
  ]
}
```

### 配置管理架構

#### 架構說明

```
ConfigLoader (Infrastructure)
    ↓ 載入 JSON 檔案
ConfigService (Domain)
    ↓ 提供業務查詢介面
Business Logic (Services/ViewModels)
    ↓ 使用配置資料
```

**職責分離**
- `ConfigLoader`：基礎設施層，負責從 Resources 載入 JSON 並反序列化
- `ConfigService`：領域層，提供業務邏輯所需的配置查詢方法

#### 配置資料類別 (Domain/Config/Models)

這些是手動定義的 C# 類別，對應 JSON 結構，使用 `[Serializable]` 讓 Unity 的 JsonUtility 可以反序列化。

```csharp
// Domain/Config/Models/GlobalConfig.cs
[Serializable]
public class GlobalConfig
{
    public string version;
    public QiSettings qi_settings;
    public RealmSettings realm_settings;
    public VipSettings vip_settings;
    public AiSettings ai_settings;
}

[Serializable]
public class QiSettings
{
    public int daily_qi_restore;
    public int max_qi;
    public int qi_per_chapter;
    public int ad_qi_restore;
}

[Serializable]
public class RealmSettings
{
    public int progress_per_chapter;
    public int max_progress;
}

[Serializable]
public class VipSettings
{
    public float monthly_price_usd;
    public int monthly_price_twd;
}

[Serializable]
public class AiSettings
{
    public string default_model;
    public int max_tokens;
    public float temperature;
}
```

```csharp
// Domain/Config/Models/RealmConfig.cs
[Serializable]
public class RealmConfig
{
    public List<RealmData> realms;
}

[Serializable]
public class RealmData
{
    public string id;
    public int order;
    public string name_key;           // Unity Localization key
    public string description_key;    // Unity Localization key
    public int required_progress;
    public float breakthrough_base_rate;
    public string next_realm;
    public ElementBonus element_bonus;
}

[Serializable]
public class ElementBonus
{
    public float metal;
    public float wood;
    public float water;
    public float fire;
    public float earth;
}
```

```csharp
// Domain/Config/Models/SpiritualRootConfig.cs
[Serializable]
public class SpiritualRootConfig
{
    public List<SpiritualRootData> spiritual_roots;
}

[Serializable]
public class SpiritualRootData
{
    public string id;
    public string name_key;           // Unity Localization key
    public string description_key;    // Unity Localization key
    public float breakthrough_modifier;
    public List<string> compatible_elements;
    public List<string> incompatible_elements;
}
```

#### 前端 ConfigLoader (Infrastructure) - 整合 Firebase Remote Config

ConfigLoader 整合 Firebase Remote Config 作為單一事實來源，支援遠端更新和版本校驗。

```csharp
// Infrastructure/Config/ConfigLoader.cs
using Firebase.RemoteConfig;
using Cysharp.Threading.Tasks;

public class ConfigLoader
{
    private static GlobalConfig _globalConfig;
    private static RealmConfig _realmConfig;
    private static SpiritualRootConfig _spiritualRootConfig;
    
    private static string _configVersion;
    private const string CONFIG_VERSION_KEY = "config_version";
    
    public static async UniTask InitializeAsync()
    {
        Debug.Log("[ConfigLoader] Starting config initialization...");
        
        // 1. 載入本地預設配置（作為 fallback）
        LoadLocalConfigs();
        
        // 2. 從 Firebase Remote Config 獲取最新配置
        await FetchRemoteConfigsAsync();
        
        // 3. 驗證配置版本
        ValidateConfigVersion();
        
        Debug.Log($"[ConfigLoader] Config initialized. Version: {_configVersion}");
    }
    
    private static void LoadLocalConfigs()
    {
        _globalConfig = LoadJson<GlobalConfig>("Configs/GlobalConfig");
        _realmConfig = LoadJson<RealmConfig>("Configs/RealmConfig");
        _spiritualRootConfig = LoadJson<SpiritualRootConfig>("Configs/SpiritualRootConfig");
    }
    
    private static async UniTask FetchRemoteConfigsAsync()
    {
        try
        {
            // 設定 Remote Config 預設值
            var defaults = new Dictionary<string, object>
            {
                { "global_config", JsonUtility.ToJson(_globalConfig) },
                { "realm_config", JsonUtility.ToJson(_realmConfig) },
                { "spiritual_root_config", JsonUtility.ToJson(_spiritualRootConfig) },
                { CONFIG_VERSION_KEY, "1.0.0" }
            };
            
            await FirebaseRemoteConfig.DefaultInstance.SetDefaultsAsync(defaults);
            
            // 從遠端獲取配置（開發環境：立即生效，生產環境：12 小時快取）
            var fetchTask = FirebaseRemoteConfig.DefaultInstance.FetchAsync(
                TimeSpan.FromHours(Application.isEditor ? 0 : 12)
            );
            
            await fetchTask;
            
            // 啟用獲取的配置
            await FirebaseRemoteConfig.DefaultInstance.ActivateAsync();
            
            // 解析遠端配置
            var remoteGlobalConfig = FirebaseRemoteConfig.DefaultInstance.GetValue("global_config").StringValue;
            var remoteRealmConfig = FirebaseRemoteConfig.DefaultInstance.GetValue("realm_config").StringValue;
            var remoteSpiritualRootConfig = FirebaseRemoteConfig.DefaultInstance.GetValue("spiritual_root_config").StringValue;
            
            // 如果遠端配置有效，覆蓋本地配置
            if (!string.IsNullOrEmpty(remoteGlobalConfig))
            {
                _globalConfig = JsonUtility.FromJson<GlobalConfig>(remoteGlobalConfig);
                Debug.Log("[ConfigLoader] Global config updated from remote");
            }
            
            if (!string.IsNullOrEmpty(remoteRealmConfig))
            {
                _realmConfig = JsonUtility.FromJson<RealmConfig>(remoteRealmConfig);
                Debug.Log("[ConfigLoader] Realm config updated from remote");
            }
            
            if (!string.IsNullOrEmpty(remoteSpiritualRootConfig))
            {
                _spiritualRootConfig = JsonUtility.FromJson<SpiritualRootConfig>(remoteSpiritualRootConfig);
                Debug.Log("[ConfigLoader] Spiritual root config updated from remote");
            }
            
            // 更新版本號
            _configVersion = FirebaseRemoteConfig.DefaultInstance.GetValue(CONFIG_VERSION_KEY).StringValue;
        }
        catch (Exception ex)
        {
            Debug.LogWarning($"[ConfigLoader] Failed to fetch remote config: {ex.Message}. Using local config.");
        }
    }
    
    private static void ValidateConfigVersion()
    {
        // 檢查配置版本是否與客戶端版本相容
        var clientVersion = Application.version;
        
        if (string.IsNullOrEmpty(_configVersion))
        {
            Debug.LogWarning("[ConfigLoader] Config version not set, using default");
            _configVersion = "1.0.0";
        }
        
        // 記錄版本資訊
        PlayerPrefs.SetString("last_config_version", _configVersion);
        PlayerPrefs.Save();
    }
    
    private static T LoadJson<T>(string path)
    {
        var textAsset = Resources.Load<TextAsset>(path);
        if (textAsset == null)
        {
            Debug.LogError($"[ConfigLoader] Failed to load config: {path}");
            return default;
        }
        
        return JsonUtility.FromJson<T>(textAsset.text);
    }
    
    // 提供配置資料的存取
    public static GlobalConfig GlobalConfig => _globalConfig;
    public static RealmConfig RealmConfig => _realmConfig;
    public static SpiritualRootConfig SpiritualRootConfig => _spiritualRootConfig;
    public static string ConfigVersion => _configVersion;
    
    // 手動重新載入配置（用於測試或強制更新）
    public static async UniTask ReloadConfigsAsync()
    {
        await InitializeAsync();
    }
}
```

#### GameEntry 初始化流程

```csharp
// Core/GameEntry.cs
private async UniTaskVoid Start()
{
    // 非同步初始化配置
    await ConfigLoader.InitializeAsync();
    
    // 配置載入完成後才初始化其他系統
    InitializeManagers();
    
    // 開啟主介面
    UIManager.Open<MainMenuView>();
}
```

**配置類別實作方式**

使用手動定義的 C# 類別對應 JSON 結構，提供型別安全和 IDE 支援。

#### 前端 ConfigService (Domain)

```csharp
// Domain/Config/ConfigService.cs
using UnityEngine.Localization;
using UnityEngine.Localization.Settings;

public class ConfigService
{
    // 境界相關查詢
    public RealmData GetRealmById(string realmId)
    {
        return ConfigLoader.RealmConfig.Realms
            .FirstOrDefault(r => r.id == realmId);
    }
    
    public string GetRealmName(string realmId)
    {
        var realm = GetRealmById(realmId);
        if (realm == null) return "Unknown";
        
        // 使用 Unity Localization 系統
        var localizedString = new LocalizedString("Game_Content", realm.name_key);
        return localizedString.GetLocalizedString();
    }
    
    public string GetRealmDescription(string realmId)
    {
        var realm = GetRealmById(realmId);
        if (realm == null) return "";
        
        var localizedString = new LocalizedString("Game_Content", realm.description_key);
        return localizedString.GetLocalizedString();
    }
    
    public RealmData GetNextRealm(string currentRealmId)
    {
        var current = GetRealmById(currentRealmId);
        if (current == null || string.IsNullOrEmpty(current.next_realm))
            return null;
        
        return GetRealmById(current.next_realm);
    }
    
    // 靈根相關查詢
    public SpiritualRootData GetSpiritualRootById(string rootId)
    {
        return ConfigLoader.SpiritualRootConfig.SpiritualRoots
            .FirstOrDefault(r => r.id == rootId);
    }
    
    public string GetSpiritualRootName(string rootId)
    {
        var root = GetSpiritualRootById(rootId);
        if (root == null) return "Unknown";
        
        var localizedString = new LocalizedString("Game_Content", root.name_key);
        return localizedString.GetLocalizedString();
    }
    
    public string GetSpiritualRootDescription(string rootId)
    {
        var root = GetSpiritualRootById(rootId);
        if (root == null) return "";
        
        var localizedString = new LocalizedString("Game_Content", root.description_key);
        return localizedString.GetLocalizedString();
    }
    
    // 突破率計算
    public float CalculateBreakthroughRate(string realmId, string spiritualRoot)
    {
        var realm = GetRealmById(realmId);
        var root = GetSpiritualRootById(spiritualRoot);
        
        if (realm == null || root == null)
            return 0.5f; // 預設 50%
        
        return realm.breakthrough_base_rate + root.breakthrough_modifier;
    }
    
    // 全域設定查詢
    public int GetDailyQiRestore()
    {
        return ConfigLoader.GlobalConfig.qi_settings.daily_qi_restore;
    }
    
    public int GetQiPerChapter()
    {
        return ConfigLoader.GlobalConfig.qi_settings.qi_per_chapter;
    }
    
    public int GetProgressPerChapter()
    {
        return ConfigLoader.GlobalConfig.realm_settings.progress_per_chapter;
    }
}
```

#### 使用範例

```csharp
// 在 GameEntry.cs 初始化時載入配置
void Awake()
{
    ConfigLoader.Initialize();
}

// 在業務邏輯中使用 ConfigService
public class CultivationService
{
    private ConfigService configService = new ConfigService();
    
    public void ReadChapter(string userId)
    {
        int qiCost = configService.GetQiPerChapter(); // 10
        int progressGain = configService.GetProgressPerChapter(); // 10
        
        // 扣除靈氣、增加進度...
    }
    
    public float CalculateBreakthroughChance(string realm, string root)
    {
        return configService.CalculateBreakthroughRate(realm, root);
    }
}

// 在 ViewModel 中使用
public class ChapterReadingViewModel
{
    private ConfigService configService = new ConfigService();
    
    public void UpdateRealmDisplay(string realmId)
    {
        // ConfigService 會自動使用當前語言從 Unity Localization 獲取文字
        string realmName = configService.GetRealmName(realmId);
        RealmNameProperty.Value = realmName;
    }
}
```

**使用流程總結**
1. GameEntry 初始化時載入所有配置（ConfigLoader）
2. 業務邏輯透過 ConfigService 查詢配置
3. ConfigService 使用 localization key 從 Unity Localization 系統獲取本地化文字
4. 配置表只儲存數值和 ID，文字內容由 Unity Localization 管理
```

#### 後端 ConfigService

```go
// internal/modules/config/config_service.go
type ConfigService struct {
    realmConfig         *RealmConfig
    spiritualRootConfig *SpiritualRootConfig
}

func (s *ConfigService) GetRealmById(realmId string) (*RealmData, error) {
    for _, realm := range s.realmConfig.Realms {
        if realm.ID == realmId {
            return &realm, nil
        }
    }
    return nil, errors.New("realm not found")
}

func (s *ConfigService) GetBreakthroughRate(realmId, spiritualRoot string) (float64, error) {
    realm, err := s.GetRealmById(realmId)
    if err != nil {
        return 0, err
    }
    
    root, err := s.GetSpiritualRootById(spiritualRoot)
    if err != nil {
        return 0, err
    }
    
    return realm.BreakthroughBaseRate + root.BreakthroughModifier, nil
}
```

## API 設計

### RPC 端點

#### 1. 命格系統（分離刷數據與 AI 生成）

**階段一：推演命格（免費、快速、可重複）**

**CalculateDestiny 請求**
```json
{
  "name": "張三",
  "gender": "male",
  "birth_date": "1990-01-01"
}
```

**CalculateDestiny 回應（< 0.5 秒）**
```json
{
  "success": true,
  "five_elements": {
    "metal": 80,
    "wood": 10,
    "water": 5,
    "fire": 3,
    "earth": 2
  },
  "spiritual_root": "metal",
  "spiritual_root_quality": "天選之人"
}
```

**業務邏輯（CalculateDestiny）**
1. 驗證輸入資料（姓名長度、日期格式）
2. 根據生辰八字計算五行屬性（純數學計算）
3. 判定靈根類型和品質
4. **不呼叫 AI，不儲存資料**
5. 立即返回數據結果
6. **AI 成本：$0**

---

**階段二：鎖定命格並生成故事（一次性 AI 生成）**

**ConfirmDestiny 請求**
```json
{
  "name": "張三",
  "gender": "male",
  "birth_date": "1990-01-01",
  "five_elements": {
    "metal": 80,
    "wood": 10,
    "water": 5,
    "fire": 3,
    "earth": 2
  },
  "spiritual_root": "metal"
}
```

**ConfirmDestiny 回應（需要等待 AI 生成）**
```json
{
  "success": true,
  "task_id": "uuid",
  "status": "generating",
  "message": "天機推演中，正在為您撰寫出身..."
}
```

**GetDestinyStatus 請求**
```json
{
  "task_id": "uuid"
}
```

**GetDestinyStatus 回應（完成）**
```json
{
  "status": "completed",
  "destiny": {
    "five_elements": {
      "metal": 80,
      "wood": 10,
      "water": 5,
      "fire": 3,
      "earth": 2
    },
    "spiritual_root": "metal",
    "destiny_description": "你乃金靈根修士，天生與金屬共鳴...",
    "origin_story": "你出生於東海鑄劍世家..."
  }
}
```

**業務邏輯（ConfirmDestiny）**
1. 驗證玩家提交的五行數據和靈根（防止竄改）
2. 重新計算五行屬性，確認與提交的數據一致
3. 創建非同步任務生成命格故事
4. **不立即呼叫 AI**，加入生成佇列
5. 返回 task_id
6. 背景 Worker 呼叫 AI 生成命格描述和出身故事
7. 儲存命格資料至 MongoDB
8. 初始化修煉資料（煉氣初期，50 靈氣）
9. **AI 成本：1 次（命格故事）**

#### 2. 讀取章節 (RequestChapter + GetChapterStatus)

**⚠️ 注意：章節生成採用非同步架構，詳見「ReadChapter 非同步架構設計」章節**

**RequestChapter 請求**
```json
{
  "chapter_number": 1,
  "request_id": "uuid"
}
```

**RequestChapter 回應（章節已存在）**
```json
{
  "status": "completed",
  "chapter": {
    "chapter_id": "uuid",
    "chapter_number": 1,
    "title": "初入修仙界",
    "content": "章節內容...",
    "qi_cost": 10,
    "realm_progress_gain": 10
  },
  "cultivation_state": {
    "current_qi": 40,
    "realm_progress": 10
  }
}
```

**RequestChapter 回應（需要生成）**
```json
{
  "status": "generating",
  "task_id": "uuid",
  "estimated_time": 10
}
```

**GetChapterStatus 請求**
```json
{
  "task_id": "uuid"
}
```

**GetChapterStatus 回應（生成中）**
```json
{
  "status": "generating",
  "task_id": "uuid",
  "progress": 0.5
}
```

**GetChapterStatus 回應（完成）**
```json
{
  "status": "completed",
  "chapter": {
    "chapter_id": "uuid",
    "chapter_number": 1,
    "title": "初入修仙界",
    "content": "章節內容..."
  },
  "cultivation_state": {
    "current_qi": 40,
    "realm_progress": 10
  }
}
```

**業務邏輯（RequestChapter）**
1. 驗證玩家靈氣是否足夠（≥10）
2. 檢查章節是否已存在（MongoDB）
3. 若已存在：
   - 扣除靈氣（-10）
   - 增加境界進度（+10%）
   - 直接返回章節內容
4. 若不存在：
   - **不扣除靈氣**（等生成成功後才扣除）
   - 檢查是否已有生成任務
   - 若無，創建新的生成任務並加入佇列
   - 返回 task_id 和狀態

**業務邏輯（GetChapterStatus）**
1. 根據 task_id 查詢生成狀態
2. 若狀態為 "completed"：
   - **此時才扣除靈氣**（-10）
   - 增加境界進度（+10%）
   - 返回章節內容
3. 若狀態為 "generating"：返回進度
4. 若狀態為 "failed"：返回錯誤，**不扣除靈氣**

#### 3. 恢復靈氣 (RestoreQi)

**請求**
```json
{
  "method": "ad|purchase",
  "amount": 10
}
```

**回應**
```json
{
  "success": true,
  "current_qi": 50,
  "message": "天地靈息已注入丹田"
}
```

**業務邏輯**
1. 驗證恢復方式（廣告或購買）
2. 若為廣告，驗證廣告觀看完成（客戶端回報）
3. 若為購買，驗證交易憑證
4. 增加靈氣（廣告 +10，購買補滿至 50）
5. 更新修煉資料
6. 記錄 Analytics 事件
7. 回傳更新後的靈氣值

#### 4. 嘗試突破 (AttemptBreakthrough + GetBreakthroughStatus)

**⚠️ 注意：突破系統採用非同步架構，避免長時間阻塞**

**AttemptBreakthrough 請求**
```json
{
  "current_realm": "煉氣初期",
  "realm_progress": 100,
  "request_id": "uuid"
}
```

**AttemptBreakthrough 回應（立即返回判定結果）**
```json
{
  "success": true,
  "breakthrough_success": true,
  "new_realm": "煉氣中期",
  "task_id": "uuid",
  "status": "generating_content",
  "message": "突破成功！正在生成悟道章節與心印閃卡..."
}
```

**AttemptBreakthrough 回應（失敗）**
```json
{
  "success": true,
  "breakthrough_success": false,
  "current_realm": "煉氣初期",
  "realm_progress": 80,
  "message": "突破失敗，境界進度重置至 80%"
}
```

**GetBreakthroughStatus 請求**
```json
{
  "task_id": "uuid"
}
```

**GetBreakthroughStatus 回應（生成中）**
```json
{
  "status": "generating",
  "progress": 0.5,
  "current_step": "generating_card"
}
```

**GetBreakthroughStatus 回應（完成）**
```json
{
  "status": "completed",
  "enlightenment_chapter": {
    "title": "悟道：天地之理",
    "content": "你在突破中領悟到..."
  },
  "heart_seal_card": {
    "image_url": "https://...",
    "realm_name": "煉氣中期",
    "date": "2025-11-12"
  }
}
```

**業務邏輯（AttemptBreakthrough）**
1. 驗證境界進度是否達到 100%
2. 查詢境界配置，獲取基礎成功率
3. 計算最終成功率（考慮命格加成）
4. 執行隨機判定
5. 若成功：
   - 更新境界至下一階段
   - 重置境界進度至 0
   - 創建非同步任務生成悟道章節和閃卡
   - **立即返回成功結果和 task_id**
6. 若失敗：
   - 境界進度重置至 80%
   - **立即返回失敗結果**（無需生成內容）
7. 記錄突破結果至資料庫
8. 記錄 Analytics 事件

**業務邏輯（GetBreakthroughStatus）**
1. 根據 task_id 查詢生成狀態
2. 返回當前進度和狀態
3. 若完成，返回悟道章節和心印閃卡


#### 5. 驗證訂閱 (VerifySubscription)

**請求**
```json
{
  "platform": "ios|android",
  "receipt": "base64_encoded_receipt"
}
```

**回應**
```json
{
  "success": true,
  "is_vip": true,
  "expiry_date": "2025-12-01T00:00:00Z"
}
```

**業務邏輯**
1. 根據平台呼叫對應的收據驗證 API（Apple/Google）
2. 驗證收據真實性
3. 解析訂閱資訊（開始日期、到期日期）
4. 更新訂閱資料至資料庫
5. 回傳 VIP 狀態

## 組件設計

### 前端組件

#### DestinyCreationView

**職責**：命格生成介面

**UI 元素**
- 姓名輸入框
- 性別選擇器（男/女/其他）
- 生日選擇器
- 確認按鈕
- 載入動畫

**互動流程**
1. 玩家輸入個人資訊
2. 點擊確認按鈕
3. 顯示載入動畫（「天機推演中...」）
4. 呼叫 CreateDestiny RPC
5. 顯示命格卡片（五行屬性、靈根、宿命描述）
6. 播放命格生成動畫
7. 自動進入章節閱讀介面

#### ChapterReadingView

**職責**：章節閱讀介面

**UI 元素**
- 境界顯示（頂部）
- 靈氣值顯示（頂部）
- 境界進度條
- 章節標題
- 章節內容（可滾動）
- 「繼續修煉」按鈕
- 「靜坐休息」按鈕
- 「吸收靈息」按鈕（觀看廣告）
- 「供奉靈石」按鈕（購買）
- 載入動畫容器
- 生成進度條

**互動流程（非同步）**
1. 顯示當前境界和靈氣值
2. 玩家點擊「繼續修煉」
3. 檢查靈氣是否足夠
4. 若足夠：
   - 顯示載入動畫（「天機推演中...」）
   - 呼叫 `ChapterService.RequestChapterAsync(chapterNumber)`
   - 若章節已存在：立即顯示內容
   - 若需要生成：
     - 訂閱生成進度事件
     - 更新進度條（「生成中... 50%」）
     - await 生成完成（輪詢 GetChapterStatus）
   - 隱藏載入動畫
   - 顯示章節內容（打字機效果）
   - 更新靈氣值和進度條
5. 若不足：
   - 顯示「靈氣微弱」提示
   - 提供恢復選項（廣告/購買/明日再修）
6. 若生成失敗：
   - 顯示錯誤提示
   - 提供重試選項
   - 靈氣不會被扣除

#### BreakthroughView

**職責**：突破介面

**UI 元素**
- 當前境界顯示
- 突破成功率顯示
- 突破按鈕
- 突破動畫容器
- 結果顯示區域
- 「悟道內容生成中」橫幅
- 生成進度指示器

**互動流程（非同步）**
1. 當境界進度達到 100% 時自動觸發
2. 顯示突破介面（「道友，突破時機已至」）
3. 顯示成功率
4. 玩家點擊突破按鈕
5. 禁用按鈕，播放點擊動畫
6. 呼叫 `BreakthroughService.AttemptBreakthroughAsync()`
7. **快速獲得突破判定結果（< 1 秒）**
8. 顯示結果：
   - **失敗**：
     - 播放失敗動畫
     - 顯示鼓勵文案（「突破失敗，境界進度重置至 80%」）
     - 重新啟用按鈕
   - **成功**：
     - 播放成功動畫（3-5 秒）
     - 顯示新境界資訊
     - 顯示「悟道內容生成中」橫幅（非阻塞）
     - 關閉突破介面，玩家可以繼續遊戲
9. **背景輪詢內容生成狀態**：
   - 呼叫 `PollBreakthroughContentAsync(taskID)`
   - 更新生成進度（「生成悟道章節... 30%」）
   - 完成後顯示通知（「悟道內容已生成」）
10. 玩家可選擇立即查看或稍後在修行歷程中查看

#### HeartSealCardView

**職責**：心印閃卡展示與分享

**UI 元素**
- 閃卡圖像顯示
- 境界名稱
- 突破日期
- 分享按鈕
- 返回按鈕

**互動流程**
1. 顯示 AI 生成的閃卡圖像
2. 玩家點擊分享按鈕
3. 儲存圖像至本地相簿
4. 開啟系統分享介面
5. 記錄分享事件至 Analytics
6. 玩家點擊返回，回到章節閱讀介面

### 後端組件

#### DestinyService

**職責**：命格生成與管理

**方法**
- `GenerateDestiny(name, gender, birthDate)`: 生成命格
- `CalculateFiveElements(birthDate)`: 計算五行屬性
- `DetermineSpiritualRoot(fiveElements)`: 確定靈根類型
- `GenerateDestinyStory(destiny, aiClient)`: 呼叫 AI 生成故事

#### ChapterService

**職責**：章節生成與管理

**方法**
- `GetOrCreateChapter(userId, chapterNumber)`: 獲取或創建章節
- `GenerateChapterContent(userId, chapterNumber, aiClient)`: 呼叫 AI 生成章節
- `ConsumeQi(userId, amount)`: 消耗靈氣
- `UpdateRealmProgress(userId, amount)`: 更新境界進度

#### BreakthroughService

**職責**：突破判定與處理

**方法**
- `AttemptBreakthrough(userId)`: 執行突破判定
- `CalculateSuccessRate(userId)`: 計算成功率
- `UpdateRealm(userId, newRealm)`: 更新境界
- `GenerateEnlightenmentChapter(userId, aiClient)`: 生成悟道章節
- `GenerateHeartSealCard(userId, realm, imageClient)`: 生成心印閃卡

#### AIGateway

**職責**：AI 服務統一閘道

**方法**
- `GenerateText(prompt, model)`: 生成文字內容
- `GenerateImage(prompt, style)`: 生成圖像
- `HandleRateLimit()`: 處理 API 限流
- `CacheResponse(key, response)`: 快取回應


## 錯誤處理

### 錯誤代碼定義

| 錯誤代碼 | 說明 | 處理方式 |
|---------|------|---------|
| `INSUFFICIENT_QI` | 靈氣不足 | 提示玩家恢復靈氣 |
| `INVALID_REALM_PROGRESS` | 境界進度不足 | 提示玩家繼續修煉 |
| `AI_SERVICE_ERROR` | AI 服務錯誤 | 重試或使用預設內容 |
| `NETWORK_ERROR` | 網路錯誤 | 自動重試 3 次 |
| `AUTHENTICATION_FAILED` | 身份驗證失敗 | 重新登入 |
| `SUBSCRIPTION_INVALID` | 訂閱無效 | 提示玩家重新訂閱 |
| `RATE_LIMIT_EXCEEDED` | API 限流 | 延遲重試 |

### 錯誤處理策略

**前端**
1. 網路錯誤：自動重試 3 次，指數退避（1s, 2s, 4s）
2. AI 生成失敗：顯示友善提示，提供重試選項
3. 資料同步失敗：暫存本地，待網路恢復後同步

**後端**
1. AI API 失敗：使用預設模板內容
2. 資料庫錯誤：記錄日誌，回傳通用錯誤
3. 驗證失敗：拒絕請求，記錄可疑行為

## 測試策略

### 單元測試

**前端 (C#)**
- DestinyService 五行計算邏輯
- CultivationService 靈氣消耗與進度計算
- BreakthroughService 成功率計算

**後端 (Go)**
- Destiny RPC 輸入驗證
- Chapter RPC 業務邏輯
- Breakthrough RPC 隨機判定

### 整合測試

**前後端整合**
- 命格生成完整流程
- 章節閱讀與靈氣消耗
- 突破判定與境界更新
- VIP 訂閱驗證

**AI 服務整合**
- OpenAI API 呼叫與回應解析
- 圖像生成 API 呼叫
- 錯誤處理與重試機制

### E2E 測試

**關鍵使用者流程**
1. 新玩家註冊 → 創建命格 → 閱讀第一章
2. 閱讀多章 → 靈氣耗盡 → 觀看廣告恢復 → 繼續閱讀
3. 境界進度滿 → 嘗試突破 → 成功 → 獲得閃卡 → 分享
4. 訂閱 VIP → 無限制閱讀

## 效能考量

### 前端優化

1. **UI Toolkit 優化**
   - 使用 USS 樣式快取
   - 避免頻繁的 UI 重建
   - 使用 Object Pool 管理 UI 元素

2. **資料快取**
   - 已讀章節快取至本地
   - 命格資料快取
   - 境界配置預載入

3. **非同步載入**
   - 使用 UniTask 避免阻塞主執行緒
   - 章節內容分段載入
   - 圖像非同步載入

### 後端優化

1. **Redis 快取策略**
   - 玩家修煉狀態快取（TTL: 1 小時）
   - 章節內容快取（TTL: 24 小時）
   - AI 生成結果快取（永久，直到更新）

2. **資料庫優化**
   - 為 user_id 建立索引
   - 使用連接池管理資料庫連接
   - 批次寫入 Analytics 事件

3. **AI API 優化**
   - 使用 Redis 快取相同 prompt 的結果
   - 實作請求佇列，避免並發過高
   - 設定合理的超時時間（30 秒）

## 安全性設計

### 身份驗證

1. **JWT Token**
   - 使用 Nakama 內建的 JWT 驗證
   - Token 有效期：7 天
   - 自動刷新機制

2. **裝置綁定**
   - 首次登入綁定裝置 ID
   - 異常登入檢測（不同裝置）

### 資料驗證

1. **輸入驗證**
   - 姓名長度限制（1-20 字元）
   - 日期格式驗證
   - 防止 SQL 注入

2. **業務邏輯驗證**
   - 伺服器端重新計算所有關鍵數值
   - 靈氣消耗驗證
   - 境界進度驗證
   - 突破條件驗證

3. **防作弊機制**
   - 客戶端時間戳驗證
   - 異常行為檢測（靈氣異常增長）
   - 請求頻率限制

### 資料加密

1. **傳輸加密**
   - 所有 API 使用 HTTPS
   - WebSocket 使用 WSS

2. **儲存加密**
   - 敏感資料（訂閱憑證）加密儲存
   - 使用 Nakama 內建的加密機制


## Firebase Analytics 事件設計

### 事件分類

**命格相關**
- `destiny_created`: 命格創建完成
  - 參數：`gender`, `spiritual_root`, `five_elements`
- `destiny_viewed`: 查看命格卡片
  - 參數：`user_id`

**章節相關**
- `chapter_read`: 閱讀章節
  - 參數：`chapter_number`, `realm`, `qi_cost`, `reading_time`
- `chapter_generated`: AI 生成章節
  - 參數：`chapter_number`, `generation_time`, `ai_model`

**靈氣相關**
- `qi_consumed`: 消耗靈氣
  - 參數：`amount`, `remaining_qi`, `action`
- `qi_restored_ad`: 觀看廣告恢復靈氣
  - 參數：`amount`, `ad_network`
- `qi_restored_purchase`: 購買恢復靈氣
  - 參數：`amount`, `price`, `currency`

**突破相關**
- `breakthrough_attempt`: 嘗試突破
  - 參數：`current_realm`, `success_rate`, `progress`
- `breakthrough_success`: 突破成功
  - 參數：`old_realm`, `new_realm`, `attempt_count`
- `breakthrough_fail`: 突破失敗
  - 參數：`realm`, `success_rate`, `attempt_count`

**分享相關**
- `card_shared`: 分享心印閃卡
  - 參數：`realm`, `platform`, `share_method`
- `card_generated`: 生成心印閃卡
  - 參數：`realm`, `generation_time`

**訂閱相關**
- `subscription_start`: 開始訂閱
  - 參數：`plan`, `price`, `platform`
- `subscription_cancel`: 取消訂閱
  - 參數：`plan`, `reason`
- `subscription_renew`: 續訂
  - 參數：`plan`, `price`

**使用者行為**
- `session_start`: 開始遊戲
  - 參數：`user_id`, `device_type`
- `session_end`: 結束遊戲
  - 參數：`duration`, `chapters_read`
- `language_changed`: 切換語言
  - 參數：`from_language`, `to_language`

## 多國語言設計

### 支援語言

1. 繁體中文 (zh-TW)
2. 簡體中文 (zh-CN)
3. 英文 (en)

### 本地化內容

**UI 文案**
- 按鈕文字
- 提示訊息
- 錯誤訊息
- 系統通知

**遊戲內容**
- 境界名稱
- 靈根類型
- 五行屬性名稱
- 系統對話（「道友」等稱呼）

**AI 生成內容**
- 根據使用者語言設定，使用對應語言的 AI prompt
- 命格描述
- 章節內容
- 悟道章節

### 實作方式

使用 Unity Localization Package：

```
Assets/Localization/
├── Tables/
│   ├── UI_Strings.asset           # UI 文案表
│   ├── Game_Content.asset         # 遊戲內容表（境界、靈根等）
│   └── System_Messages.asset      # 系統訊息表
└── Settings/
    └── Localization Settings.asset
```

#### Game_Content 本地化表範例

| Key | 繁體中文 (zh-TW) | 簡體中文 (zh-CN) | English (en) |
|-----|-----------------|-----------------|--------------|
| `realm.qi_refining_early.name` | 煉氣初期 | 炼气初期 | Qi Refining - Early |
| `realm.qi_refining_early.description` | 修仙之路的起點，感知天地靈氣 | 修仙之路的起点，感知天地灵气 | The beginning of cultivation, sensing spiritual energy |
| `realm.qi_refining_mid.name` | 煉氣中期 | 炼气中期 | Qi Refining - Mid |
| `realm.qi_refining_mid.description` | 靈氣運轉漸趨純熟 | 灵气运转渐趋纯熟 | Spiritual energy circulation becomes proficient |
| `spiritual_root.metal.name` | 金靈根 | 金灵根 | Metal Root |
| `spiritual_root.metal.description` | 金主剛強，修煉速度快但易遇瓶頸 | 金主刚强，修炼速度快但易遇瓶颈 | Metal represents strength, fast cultivation but prone to bottlenecks |
| `spiritual_root.wood.name` | 木靈根 | 木灵根 | Wood Root |
| `spiritual_root.wood.description` | 木主生機，修煉穩健持久 | 木主生机，修炼稳健持久 | Wood represents vitality, steady and enduring cultivation |

**語言切換流程**
1. 偵測系統語言（首次啟動）
2. 載入對應的本地化表
3. 更新所有 UI 文字
4. 儲存語言偏好至本地
5. 通知 AI 服務使用對應語言

## 部署架構

### 開發環境

```
┌─────────────────┐
│  Unity Editor   │
│  (localhost)    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Nakama Local    │
│ Docker Compose  │
│ localhost:7350  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  PostgreSQL     │
│  localhost:5432 │
└─────────────────┘
```

### 測試環境

```
┌─────────────────┐
│  Unity Build    │
│  (TestFlight/   │
│   Internal)     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Nakama Staging  │
│ staging.api.xxx │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  PostgreSQL     │
│  (Cloud)        │
└─────────────────┘
```

### 生產環境

```
┌─────────────────┐
│  Unity Build    │
│  (App Store/    │
│   Google Play)  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Nakama Prod     │
│ api.xxx.com     │
│ (Load Balanced) │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  PostgreSQL     │
│  (Replicated)   │
└─────────────────┘
```

## 監控與日誌

### 監控指標

**系統指標**
- API 回應時間
- 錯誤率
- 並發使用者數
- 資料庫連接數

**業務指標**
- DAU (每日活躍用戶)
- 平均遊戲時長
- 章節閱讀數
- 突破成功率
- VIP 轉化率
- 廣告觀看率

**AI 服務指標**
- API 呼叫次數
- 平均生成時間
- 錯誤率
- 成本追蹤

### 日誌策略

**前端日誌**
- 使用 Unity Debug.Log
- 關鍵操作記錄（RPC 呼叫、錯誤）
- 本地日誌檔案（最多保留 7 天）

**後端日誌**
- 使用 Nakama Logger
- 結構化日誌（JSON 格式）
- 日誌等級：Debug, Info, Warn, Error
- 集中式日誌收集（ELK Stack 或 CloudWatch）

## 未來擴展考量

### v2.0 功能

1. **宗門系統**
   - 玩家可創建或加入宗門
   - 宗門成員間互贈靈氣
   - 宗門排行榜

2. **社交功能**
   - 好友系統
   - 修煉進度分享
   - 悟道心得留言

3. **更多境界**
   - 擴展至金丹期、元嬰期
   - 每個境界獨特的故事線

### v3.0 功能

1. **放置修煉**
   - 自動積累靈氣
   - 定時突破提醒
   - 離線收益

2. **多平台支援**
   - Steam 版本
   - 網頁版本
   - 跨平台進度同步

### 技術債務管理

1. **程式碼品質**
   - 定期 Code Review
   - 單元測試覆蓋率 > 70%
   - 使用 SonarQube 進行靜態分析

2. **效能優化**
   - 定期效能測試
   - 記憶體洩漏檢測
   - 資料庫查詢優化

3. **文件維護**
   - API 文件自動生成
   - 架構圖定期更新
   - 變更日誌記錄



## AI 系統與成本管理

### AI 成本追蹤

為了控制 AI 生成成本，需要建立完整的追蹤機制。

#### AI 使用記錄表 (MongoDB)

```json
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "request_type": "destiny|chapter|enlightenment|card",
  "ai_model": "gpt-4-mini",
  "prompt_hash": "sha256_hash",
  "prompt_version": "v1.2",
  "tokens_used": {
    "prompt": 150,
    "completion": 450,
    "total": 600
  },
  "cost_usd": 0.0012,
  "generation_time_ms": 2500,
  "success": true,
  "created_at": "2025-11-12T10:00:00Z"
}
```

**索引**
- `created_at`: 用於成本統計
- `user_id` + `created_at`: 用於使用者成本分析
- `ai_model` + `created_at`: 用於模型成本比較

#### 成本追蹤服務

```go
// pkg/ai/cost_tracker.go
type CostTracker struct {
    db *mongo.Database
}

func (t *CostTracker) RecordUsage(ctx context.Context, record AIUsageRecord) error {
    collection := t.db.Collection("ai_usage")
    _, err := collection.InsertOne(ctx, record)
    return err
}

func (t *CostTracker) GetDailyCost(ctx context.Context, date time.Time) (float64, error) {
    // 查詢當日總成本
    pipeline := []bson.M{
        {"$match": bson.M{
            "created_at": bson.M{
                "$gte": date,
                "$lt": date.Add(24 * time.Hour),
            },
        }},
        {"$group": bson.M{
            "_id": nil,
            "total_cost": bson.M{"$sum": "$cost_usd"},
        }},
    }
    // ...
}
```

### AI 快取策略優化

#### Redis 快取結構

```
Key: ai_cache:{prompt_hash}:{model_version}
Value: {
  "content": "生成的內容",
  "tokens": 600,
  "cost": 0.0012,
  "created_at": "2025-11-12T10:00:00Z",
  "version": "v1.2"
}
TTL: 永久（手動清理舊版本）
```

#### Prompt 版本管理

```go
// pkg/ai/prompt_manager.go
type PromptTemplate struct {
    ID          string
    Version     string
    Template    string
    Parameters  []string
    Language    string
    UpdatedAt   time.Time
}

type PromptManager struct {
    templates map[string]*PromptTemplate
}

func (m *PromptManager) GetPrompt(templateID, language string, params map[string]string) (string, string) {
    template := m.templates[templateID]
    
    // 替換參數
    prompt := template.Template
    for key, value := range params {
        prompt = strings.ReplaceAll(prompt, "{"+key+"}", value)
    }
    
    // 計算 hash（包含版本）
    hash := sha256.Sum256([]byte(prompt + template.Version))
    promptHash := hex.EncodeToString(hash[:])
    
    return prompt, promptHash
}
```

#### Prompt 模板範例

```json
{
  "templates": [
    {
      "id": "destiny_description",
      "version": "v1.2",
      "language": "zh-TW",
      "template": "你是一位修仙世界的命理大師。根據以下資訊生成命格描述：\n姓名：{name}\n性別：{gender}\n五行屬性：{elements}\n靈根：{spiritual_root}\n\n請用詩意、神秘的語氣描述此人的修仙宿命，字數 200-300 字。",
      "parameters": ["name", "gender", "elements", "spiritual_root"]
    },
    {
      "id": "chapter_content",
      "version": "v1.3",
      "language": "zh-TW",
      "template": "你是一位修仙小說作家。根據以下資訊生成章節內容：\n主角：{name}\n當前境界：{realm}\n章節編號：{chapter_number}\n命格特徵：{destiny_traits}\n\n請生成一個修煉故事章節，包含修煉場景、心境描寫和境界感悟，字數 500-800 字。保持與前文連貫。",
      "parameters": ["name", "realm", "chapter_number", "destiny_traits"]
    }
  ]
}
```

### AI 生成節奏控制

#### 非同步生成佇列

```go
// internal/modules/chapter/generation_queue.go
type GenerationQueue struct {
    redis *redis.Client
}

func (q *GenerationQueue) EnqueueChapter(userID string, chapterNumber int) error {
    task := GenerationTask{
        UserID:        userID,
        ChapterNumber: chapterNumber,
        Priority:      "normal",
        CreatedAt:     time.Now(),
    }
    
    // 加入 Redis 佇列
    return q.redis.LPush(context.Background(), "generation_queue", task).Err()
}

func (q *GenerationQueue) ProcessQueue() {
    for {
        // 從佇列取出任務
        result := q.redis.BRPop(context.Background(), 0, "generation_queue")
        
        // 處理生成任務
        // 控制並發數（例如最多 5 個同時生成）
        // ...
    }
}
```

#### 預載下一章策略

```go
func (s *ChapterService) PreloadNextChapter(userID string, currentChapter int) {
    // 檢查下一章是否已存在
    nextChapter := currentChapter + 1
    exists := s.CheckChapterExists(userID, nextChapter)
    
    if !exists {
        // 加入低優先級佇列
        s.queue.EnqueueChapter(userID, nextChapter)
    }
}
```



## 資料架構優化

### 版本控制與 Migration

#### MongoDB Collection 版本欄位

所有 collection 加入版本控制欄位：

```json
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "schema_version": "1.0",
  "last_migrated_at": "2025-11-12T00:00:00Z",
  // ... 其他欄位
}
```

#### Migration 工具

使用 `golang-migrate` 管理資料庫 schema 變更：

```bash
# 安裝
go get -u github.com/golang-migrate/migrate/v4

# 建立 migration
migrate create -ext json -dir migrations -seq add_version_field
```

Migration 範例：

```go
// migrations/000001_add_version_field.up.go
db.Collection("players").UpdateMany(
    context.Background(),
    bson.M{"schema_version": bson.M{"$exists": false}},
    bson.M{"$set": bson.M{
        "schema_version": "1.0",
        "last_migrated_at": time.Now(),
    }},
)
```

### 資料清理策略

#### 自動清理規則

```go
// pkg/database/cleanup_service.go
type CleanupService struct {
    db *mongo.Database
}

func (s *CleanupService) CleanupInactiveUsers() error {
    // 刪除 30 天未登入玩家的暫存章節
    thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
    
    // 找出不活躍玩家
    inactiveUsers, err := s.db.Collection("players").Find(
        context.Background(),
        bson.M{"last_login": bson.M{"$lt": thirtyDaysAgo}},
    )
    
    // 刪除章節快取
    for _, user := range inactiveUsers {
        s.db.Collection("chapters").DeleteMany(
            context.Background(),
            bson.M{
                "user_id": user.UserID,
                "chapter_type": "normal", // 保留突破章節
            },
        )
    }
    
    return nil
}
```

#### 定期執行清理

```go
// 在 Nakama 啟動時註冊定時任務
func RegisterCleanupJobs(nk runtime.NakamaModule) {
    // 每天凌晨 3 點執行清理
    nk.CronNext("0 3 * * *", func() {
        cleanupService.CleanupInactiveUsers()
    })
}
```

### 前端離線快取與同步

#### 離線資料結構

```csharp
// Infrastructure/Storage/OfflineCache.cs
[Serializable]
public class OfflineCacheData
{
    public string user_id;
    public long last_sync_timestamp;
    public List<CachedChapter> chapters;
    public CultivationState cultivation_state;
    public int cache_version;
}

[Serializable]
public class CachedChapter
{
    public int chapter_number;
    public string content;
    public long cached_at;
    public bool is_synced;
}
```

#### 同步策略

```csharp
// Infrastructure/Storage/SyncManager.cs
public class SyncManager
{
    public async UniTask SyncWithServer()
    {
        var localData = LoadOfflineCache();
        var serverTimestamp = await NetworkManager.Instance.GetServerTimestamp();
        
        if (localData.last_sync_timestamp < serverTimestamp)
        {
            // 伺服器資料較新，下載更新
            var serverData = await NetworkManager.Instance.GetPlayerData();
            MergeData(localData, serverData);
        }
        else
        {
            // 本地資料較新，上傳更新
            await NetworkManager.Instance.UpdatePlayerData(localData);
        }
        
        SaveOfflineCache(localData);
    }
    
    private void MergeData(OfflineCacheData local, PlayerData server)
    {
        // 使用時間戳判斷哪個版本較新
        if (server.cultivation_state.updated_at > local.cultivation_state.updated_at)
        {
            local.cultivation_state = server.cultivation_state;
        }
        
        // 合併章節（保留本地未同步的章節）
        foreach (var serverChapter in server.chapters)
        {
            var localChapter = local.chapters.Find(c => c.chapter_number == serverChapter.chapter_number);
            if (localChapter == null || serverChapter.created_at > localChapter.cached_at)
            {
                local.chapters.Add(new CachedChapter
                {
                    chapter_number = serverChapter.chapter_number,
                    content = serverChapter.content,
                    cached_at = DateTimeOffset.UtcNow.ToUnixTimeSeconds(),
                    is_synced = true
                });
            }
        }
    }
}
```



## 故事系統與體驗設計

### 故事弧線（Story Arc）系統

為了讓 AI 生成的章節具有連貫性和主題感，引入故事弧線機制。

#### 故事弧線配置

```json
{
  "story_arcs": [
    {
      "id": "arc_1_awakening",
      "name": "覺醒之路",
      "realm_range": ["qi_refining_early", "qi_refining_mid"],
      "chapter_range": [1, 10],
      "theme": "初入修仙界，感知靈氣，建立修煉基礎",
      "key_events": [
        {
          "chapter": 3,
          "event": "首次感知靈氣"
        },
        {
          "chapter": 7,
          "event": "遇見引路人"
        },
        {
          "chapter": 10,
          "event": "第一次突破前的心魔考驗"
        }
      ]
    },
    {
      "id": "arc_2_foundation",
      "name": "築基之路",
      "realm_range": ["qi_refining_late", "foundation_early"],
      "chapter_range": [11, 25],
      "theme": "穩固根基，領悟修煉真諦",
      "key_events": [
        {
          "chapter": 15,
          "event": "獲得功法傳承"
        },
        {
          "chapter": 20,
          "event": "面對第一個敵人"
        }
      ]
    }
  ]
}
```

#### AI Prompt 整合故事弧線

```go
func (s *ChapterService) GenerateChapterWithArc(userID string, chapterNumber int) (string, error) {
    // 查詢當前故事弧線
    arc := s.configService.GetStoryArcByChapter(chapterNumber)
    
    // 檢查是否有關鍵事件
    keyEvent := arc.GetKeyEvent(chapterNumber)
    
    // 構建 prompt
    promptParams := map[string]string{
        "name": player.Name,
        "realm": player.CurrentRealm,
        "chapter_number": strconv.Itoa(chapterNumber),
        "arc_theme": arc.Theme,
        "key_event": keyEvent, // 如果有關鍵事件，加入 prompt
    }
    
    prompt, hash := s.promptManager.GetPrompt("chapter_with_arc", player.Language, promptParams)
    
    // 呼叫 AI 生成
    content, err := s.aiGateway.GenerateText(prompt, "gpt-4-mini")
    return content, err
}
```

### 修行儀式與每日任務

#### 每日修行任務配置

```json
{
  "daily_rituals": [
    {
      "id": "morning_meditation",
      "name": {
        "zh_tw": "晨間靜坐",
        "zh_cn": "晨间静坐",
        "en": "Morning Meditation"
      },
      "description": {
        "zh_tw": "清晨靜坐一刻鐘，感悟天地靈氣",
        "zh_cn": "清晨静坐一刻钟，感悟天地灵气",
        "en": "Meditate for 15 minutes at dawn"
      },
      "reward": {
        "qi": 5,
        "progress": 2
      },
      "cooldown_hours": 24
    },
    {
      "id": "read_three_chapters",
      "name": {
        "zh_tw": "日讀三章",
        "zh_cn": "日读三章",
        "en": "Read Three Chapters"
      },
      "description": {
        "zh_tw": "每日閱讀三章修煉心得",
        "zh_cn": "每日阅读三章修炼心得",
        "en": "Read three chapters daily"
      },
      "requirement": {
        "type": "read_chapters",
        "count": 3
      },
      "reward": {
        "qi": 10,
        "progress": 5
      }
    }
  ]
}
```

#### 每日任務追蹤

```json
// MongoDB: daily_rituals collection
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "ritual_id": "morning_meditation",
  "completed": true,
  "completed_at": "2025-11-12T06:00:00Z",
  "reward_claimed": true,
  "reset_at": "2025-11-13T00:00:00Z"
}
```

### 新手引導（Onboarding）

#### 入道指引流程

```
1. 命格生成動畫（3 秒）
   ↓
2. 命格卡片展示（玩家可查看五行屬性）
   ↓
3. 「入道指引」特殊章節（AI 生成，介紹修仙世界觀）
   ↓
4. 首次修煉教學（引導閱讀第一章）
   ↓
5. 靈氣系統說明（動畫展示靈氣消耗與恢復）
   ↓
6. 完成新手引導，獲得獎勵（+20 靈氣）
```

#### 新手引導資料

```json
// MongoDB: onboarding_progress collection
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "current_step": 3,
  "completed_steps": ["destiny_created", "destiny_viewed", "intro_chapter_read"],
  "completed": false,
  "started_at": "2025-11-12T10:00:00Z"
}
```

### 社群分享優化

#### 修行歷程卡

除了單次突破的心印閃卡，加入「修行歷程卡」功能：

```csharp
// Domain/Share/CultivationJourneyCard.cs
public class CultivationJourneyCard
{
    public string UserName;
    public List<RealmMilestone> Milestones;
    public int TotalChaptersRead;
    public int TotalDays;
    public string CurrentRealm;
    
    public Texture2D GenerateCard()
    {
        // 生成包含多個境界的歷程圖
        // 顯示修煉天數、閱讀章節數、突破次數等
        // 使用時間軸視覺呈現
    }
}
```

#### 年度總結

```csharp
// 在特定時間點（如年底）生成年度總結
public class AnnualSummary
{
    public int TotalChaptersRead;
    public int TotalBreakthroughs;
    public string HighestRealm;
    public List<string> KeyMoments; // AI 生成的關鍵時刻摘要
    public Dictionary<string, int> ElementDistribution; // 五行修煉分布
}
```



## 安全性強化

### IAP 驗證與簽章

#### 交易簽章機制

```go
// pkg/payment/signature.go
func GenerateTransactionSignature(userID, transactionID, timestamp, secret string) string {
    data := fmt.Sprintf("%s:%s:%s:%s", userID, transactionID, timestamp, secret)
    hash := hmac.New(sha256.New, []byte(secret))
    hash.Write([]byte(data))
    return hex.EncodeToString(hash.Sum(nil))
}

func VerifyTransactionSignature(userID, transactionID, timestamp, signature, secret string) bool {
    expected := GenerateTransactionSignature(userID, transactionID, timestamp, secret)
    return hmac.Equal([]byte(signature), []byte(expected))
}
```

#### 前端交易請求

```csharp
// 購買靈石時
public async UniTask<bool> PurchaseSpiritStone(int amount)
{
    var timestamp = DateTimeOffset.UtcNow.ToUnixTimeSeconds().ToString();
    var transactionID = GenerateTransactionID();
    
    // 計算簽章（使用本地密鑰）
    var signature = GenerateSignature(userId, transactionID, timestamp);
    
    var request = new PurchaseRequest
    {
        user_id = userId,
        transaction_id = transactionID,
        amount = amount,
        timestamp = timestamp,
        signature = signature,
        platform_receipt = receipt
    };
    
    var response = await NetworkManager.Instance.RpcAsync<PurchaseResponse>("PurchaseSpiritStone", request);
    return response.success;
}
```

#### 後端驗證

```go
func RpcPurchaseSpiritStone(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    var req PurchaseRequest
    json.Unmarshal([]byte(payload), &req)
    
    // 1. 驗證簽章
    if !VerifyTransactionSignature(req.UserID, req.TransactionID, req.Timestamp, req.Signature, SECRET_KEY) {
        return "", errors.New("invalid signature")
    }
    
    // 2. 驗證時間戳（防止重放攻擊）
    timestamp, _ := strconv.ParseInt(req.Timestamp, 10, 64)
    if time.Now().Unix()-timestamp > 300 { // 5 分鐘內有效
        return "", errors.New("timestamp expired")
    }
    
    // 3. 驗證交易 ID 唯一性
    exists := CheckTransactionExists(req.TransactionID)
    if exists {
        return "", errors.New("duplicate transaction")
    }
    
    // 4. 驗證平台收據
    valid, err := VerifyPlatformReceipt(req.PlatformReceipt, req.Platform)
    if !valid {
        return "", errors.New("invalid receipt")
    }
    
    // 5. 處理交易
    // ...
}
```

### 行為異常偵測

#### 異常行為記錄

```json
// MongoDB: suspicious_activities collection
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "activity_type": "qi_anomaly|frequent_rpc|invalid_request",
  "details": {
    "expected_value": 50,
    "actual_value": 150,
    "delta": 100
  },
  "severity": "low|medium|high",
  "detected_at": "2025-11-12T10:00:00Z",
  "resolved": false
}
```

#### 異常偵測規則

```go
// pkg/security/anomaly_detector.go
type AnomalyDetector struct {
    redis *redis.Client
}

func (d *AnomalyDetector) CheckQiAnomaly(userID string, reportedQi, expectedQi float64) bool {
    delta := math.Abs(reportedQi - expectedQi)
    threshold := expectedQi * 0.05 // 5% 容錯
    
    if delta > threshold {
        d.RecordSuspiciousActivity(userID, "qi_anomaly", map[string]interface{}{
            "expected": expectedQi,
            "reported": reportedQi,
            "delta": delta,
        })
        return true
    }
    return false
}

func (d *AnomalyDetector) CheckRPCFrequency(userID string) bool {
    key := fmt.Sprintf("rpc_count:%s", userID)
    count, _ := d.redis.Incr(context.Background(), key).Result()
    d.redis.Expire(context.Background(), key, time.Minute)
    
    // 每分鐘超過 60 次 RPC 視為異常
    if count > 60 {
        d.RecordSuspiciousActivity(userID, "frequent_rpc", map[string]interface{}{
            "count": count,
            "window": "1 minute",
        })
        return true
    }
    return false
}
```

### 請求完整性驗證

#### Nakama RPC 請求簽章

```go
// pkg/middleware/request_validator.go
func ValidateRequest(ctx context.Context, payload string, signature string) error {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    timestamp := ctx.Value("timestamp").(string)
    
    // 重新計算簽章
    expected := GenerateRequestSignature(userID, payload, timestamp, SECRET_KEY)
    
    if !hmac.Equal([]byte(signature), []byte(expected)) {
        return errors.New("invalid request signature")
    }
    
    return nil
}

// 在所有 RPC 中加入驗證
func RpcReadChapter(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    signature := ctx.Value("signature").(string)
    
    if err := ValidateRequest(ctx, payload, signature); err != nil {
        logger.Warn("Invalid request signature", "error", err)
        return "", err
    }
    
    // 正常處理邏輯
    // ...
}
```

### 日誌可追溯性

#### 結構化日誌格式

```go
// pkg/logging/structured_logger.go
type StructuredLogger struct {
    logger runtime.Logger
}

func (l *StructuredLogger) LogUserAction(userID, sessionID, action string, details map[string]interface{}) {
    logEntry := map[string]interface{}{
        "timestamp": time.Now().UTC().Format(time.RFC3339),
        "user_id": userID,
        "session_id": sessionID,
        "action": action,
        "details": details,
    }
    
    jsonLog, _ := json.Marshal(logEntry)
    l.logger.Info(string(jsonLog))
}

// 使用範例
logger.LogUserAction(userID, sessionID, "breakthrough_attempt", map[string]interface{}{
    "realm": "qi_refining_early",
    "success_rate": 0.75,
    "result": "success",
})
```

#### 關鍵操作日誌

所有關鍵操作都需要記錄：

```go
// 突破
logger.LogUserAction(userID, sessionID, "breakthrough", details)

// 支付
logger.LogUserAction(userID, sessionID, "purchase", details)

// AI 生成
logger.LogUserAction(userID, sessionID, "ai_generation", details)

// 訂閱
logger.LogUserAction(userID, sessionID, "subscription", details)
```



## 分析與營運優化

### Firebase Analytics 進階事件

#### 留存漏斗事件

```csharp
// 章節深度追蹤
AnalyticsManager.Instance.LogEvent("chapter_depth", new {
    chapter_number = chapterNumber,
    realm = currentRealm,
    session_duration = sessionDuration,
    is_first_session = isFirstSession
});

// 回訪間隔追蹤
AnalyticsManager.Instance.LogEvent("return_interval", new {
    days_since_last_visit = daysSinceLastVisit,
    total_sessions = totalSessions,
    retention_cohort = retentionCohort // "day_1", "day_7", "day_30"
});

// 內容偏好追蹤
AnalyticsManager.Instance.LogEvent("content_preference", new {
    chapter_type = chapterType, // "normal", "breakthrough", "enlightenment"
    reading_speed = readingSpeed, // 字數/秒
    completion_rate = completionRate // 是否讀完
});
```

#### 行為指標事件

```csharp
// 靈氣使用模式
AnalyticsManager.Instance.LogEvent("qi_usage_pattern", new {
    restore_method = "daily|ad|purchase",
    qi_efficiency = qiUsed / chaptersRead,
    session_qi_consumption = sessionQiConsumption
});

// 突破行為
AnalyticsManager.Instance.LogEvent("breakthrough_behavior", new {
    attempts_before_success = attemptsBeforeSuccess,
    time_in_realm = timeInRealm, // 秒數
    preparation_chapters = preparationChapters // 突破前閱讀的章節數
});
```

### 成本效益儀表板

#### 成本收益追蹤表 (MongoDB)

```json
{
  "_id": "ObjectId",
  "date": "2025-11-12",
  "metrics": {
    "ai_cost": {
      "total_usd": 125.50,
      "by_model": {
        "gpt-4-mini": 80.30,
        "claude-haiku": 25.20,
        "stable-diffusion": 20.00
      },
      "by_type": {
        "destiny": 15.00,
        "chapter": 95.00,
        "enlightenment": 10.50,
        "card": 5.00
      }
    },
    "revenue": {
      "total_usd": 450.00,
      "vip_subscriptions": 350.00,
      "spirit_stone_purchases": 100.00
    },
    "user_metrics": {
      "dau": 3500,
      "new_users": 250,
      "vip_users": 180
    }
  },
  "roi": 2.58 // revenue / ai_cost
}
```

#### 成本效益分析服務

```go
// pkg/analytics/cost_analysis.go
type CostAnalysisService struct {
    db *mongo.Database
}

func (s *CostAnalysisService) GenerateDailyReport(date time.Time) (*DailyReport, error) {
    // 查詢 AI 成本
    aiCost := s.GetAICost(date)
    
    // 查詢收益
    revenue := s.GetRevenue(date)
    
    // 查詢使用者指標
    userMetrics := s.GetUserMetrics(date)
    
    // 計算 ROI
    roi := revenue.Total / aiCost.Total
    
    report := &DailyReport{
        Date: date,
        AICost: aiCost,
        Revenue: revenue,
        UserMetrics: userMetrics,
        ROI: roi,
    }
    
    // 儲存報表
    s.db.Collection("daily_reports").InsertOne(context.Background(), report)
    
    return report, nil
}

func (s *CostAnalysisService) GetCostPerUser(date time.Time) float64 {
    report := s.GetDailyReport(date)
    return report.AICost.Total / float64(report.UserMetrics.DAU)
}

func (s *CostAnalysisService) GetLTV(userID string) float64 {
    // 計算使用者生命週期價值
    totalRevenue := s.GetUserTotalRevenue(userID)
    totalCost := s.GetUserTotalAICost(userID)
    return totalRevenue - totalCost
}
```

### MVP 驗證指標

#### 關鍵 KPI 定義

```json
{
  "mvp_kpis": {
    "retention": {
      "day_1": {
        "target": 0.40,
        "minimum": 0.30,
        "description": "次日留存率"
      },
      "day_7": {
        "target": 0.30,
        "minimum": 0.20,
        "description": "7 日留存率"
      },
      "day_30": {
        "target": 0.15,
        "minimum": 0.10,
        "description": "30 日留存率"
      }
    },
    "engagement": {
      "avg_session_duration": {
        "target": 300,
        "minimum": 180,
        "unit": "seconds",
        "description": "平均遊戲時長"
      },
      "chapters_per_session": {
        "target": 3,
        "minimum": 2,
        "description": "每次遊戲閱讀章節數"
      }
    },
    "monetization": {
      "vip_conversion_rate": {
        "target": 0.05,
        "minimum": 0.03,
        "description": "VIP 轉化率"
      },
      "arpu": {
        "target": 0.50,
        "minimum": 0.30,
        "unit": "usd",
        "description": "平均每使用者收益"
      }
    },
    "content": {
      "breakthrough_success_rate": {
        "target": 0.70,
        "minimum": 0.60,
        "description": "突破成功率"
      },
      "ai_generation_success_rate": {
        "target": 0.98,
        "minimum": 0.95,
        "description": "AI 生成成功率"
      }
    }
  }
}
```

#### KPI 監控儀表板

```go
// pkg/analytics/kpi_monitor.go
type KPIMonitor struct {
    db *mongo.Database
    config *KPIConfig
}

func (m *KPIMonitor) CheckKPIs(date time.Time) (*KPIReport, error) {
    report := &KPIReport{
        Date: date,
        Status: "healthy",
        Alerts: []string{},
    }
    
    // 檢查留存率
    day1Retention := m.CalculateRetention(date, 1)
    if day1Retention < m.config.Retention.Day1.Minimum {
        report.Status = "warning"
        report.Alerts = append(report.Alerts, fmt.Sprintf("Day 1 retention below minimum: %.2f%%", day1Retention*100))
    }
    
    // 檢查 VIP 轉化率
    vipConversion := m.CalculateVIPConversion(date)
    if vipConversion < m.config.Monetization.VIPConversionRate.Minimum {
        report.Status = "warning"
        report.Alerts = append(report.Alerts, fmt.Sprintf("VIP conversion below minimum: %.2f%%", vipConversion*100))
    }
    
    // 檢查 AI 成本效益
    roi := m.CalculateROI(date)
    if roi < 1.0 {
        report.Status = "critical"
        report.Alerts = append(report.Alerts, fmt.Sprintf("ROI below 1.0: %.2f", roi))
    }
    
    return report, nil
}
```

### 使用者分層與 A/B 測試

#### 使用者分層

```json
// MongoDB: user_segments collection
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "segments": [
    "new_user",      // 註冊 < 7 天
    "active_reader", // 每日閱讀 > 3 章
    "potential_vip", // 未訂閱但活躍度高
    "whale"          // 高消費使用者
  ],
  "ab_test_groups": {
    "qi_consumption_test": "group_a", // 每章消耗 10 靈氣
    "chapter_length_test": "group_b"  // 章節長度 800 字
  },
  "updated_at": "2025-11-12T10:00:00Z"
}
```

#### A/B 測試配置

```json
{
  "ab_tests": [
    {
      "id": "qi_consumption_test",
      "name": "靈氣消耗測試",
      "status": "active",
      "start_date": "2025-11-01",
      "end_date": "2025-11-30",
      "groups": [
        {
          "id": "group_a",
          "name": "控制組",
          "config": {
            "qi_per_chapter": 10
          },
          "allocation": 0.5
        },
        {
          "id": "group_b",
          "name": "實驗組",
          "config": {
            "qi_per_chapter": 8
          },
          "allocation": 0.5
        }
      ],
      "metrics": ["retention_day_7", "chapters_per_session", "vip_conversion"]
    }
  ]
}
```

#### A/B 測試服務

```go
// pkg/abtest/ab_test_service.go
type ABTestService struct {
    db *mongo.Database
}

func (s *ABTestService) AssignUserToGroup(userID string, testID string) string {
    // 使用使用者 ID hash 確保一致性分配
    hash := md5.Sum([]byte(userID + testID))
    hashInt := binary.BigEndian.Uint64(hash[:])
    
    test := s.GetABTest(testID)
    random := float64(hashInt%10000) / 10000.0
    
    cumulative := 0.0
    for _, group := range test.Groups {
        cumulative += group.Allocation
        if random < cumulative {
            return group.ID
        }
    }
    
    return test.Groups[0].ID // 預設返回第一組
}

func (s *ABTestService) GetConfigForUser(userID string, testID string) map[string]interface{} {
    groupID := s.AssignUserToGroup(userID, testID)
    test := s.GetABTest(testID)
    
    for _, group := range test.Groups {
        if group.ID == groupID {
            return group.Config
        }
    }
    
    return nil
}
```

## MVP 驗證總結

### 成功標準

MVP 被視為成功需要達到以下標準：

1. **留存率**
   - Day 1 留存率 ≥ 30%
   - Day 7 留存率 ≥ 20%

2. **參與度**
   - 平均遊戲時長 ≥ 3 分鐘
   - 每次遊戲閱讀 ≥ 2 章

3. **營收**
   - VIP 轉化率 ≥ 3%
   - AI 成本 ROI ≥ 1.5

4. **技術穩定性**
   - API 成功率 ≥ 99%
   - AI 生成成功率 ≥ 95%

### 優化迭代計畫

根據 MVP 數據決定下一步：

- **若留存率低**：優化故事內容、加強新手引導
- **若轉化率低**：調整 VIP 價格、增加特權吸引力
- **若成本過高**：優化 AI prompt、增加快取命中率
- **若參與度低**：調整靈氣節奏、增加每日任務



## API 冪等性機制（Idempotency）

### 問題說明

在網路不穩定的情況下，客戶端可能會重複發送相同的請求（如閱讀章節、突破境界），導致：
- 靈氣被重複扣除
- 境界進度被重複增加
- 突破被重複執行

### 解決方案

為所有有副作用的 RPC 請求加入唯一的 `request_id`，伺服器端檢查是否已執行過，若重複則直接返回快取結果。

### 實作設計

#### 前端請求結構

```csharp
// Infrastructure/Network/RpcPayloads.cs
[Serializable]
public class IdempotentRequest
{
    public string request_id;  // UUID，客戶端生成
    public long timestamp;     // 請求時間戳
}

[Serializable]
public class ReadChapterRequest : IdempotentRequest
{
    public int chapter_number;
}

[Serializable]
public class AttemptBreakthroughRequest : IdempotentRequest
{
    public string current_realm;
    public float realm_progress;
}

[Serializable]
public class RestoreQiRequest : IdempotentRequest
{
    public string method; // "ad" | "purchase"
    public int amount;
}
```

#### 前端請求生成

```csharp
// Infrastructure/Network/NetworkManager.cs
public async UniTask<T> RpcWithIdempotency<T>(string rpcId, IdempotentRequest payload)
{
    // 生成唯一 request_id
    payload.request_id = System.Guid.NewGuid().ToString();
    payload.timestamp = DateTimeOffset.UtcNow.ToUnixTimeSeconds();
    
    return await RpcAsync<T>(rpcId, payload);
}
```

#### 後端冪等性檢查

```go
// pkg/middleware/idempotency.go
type IdempotencyMiddleware struct {
    redis *redis.Client
}

func (m *IdempotencyMiddleware) CheckIdempotency(ctx context.Context, userID, requestID string) (string, bool, error) {
    key := fmt.Sprintf("idempotency:%s:%s", userID, requestID)
    
    // 檢查是否已執行過
    cachedResult, err := m.redis.Get(ctx, key).Result()
    if err == nil {
        // 已執行過，返回快取結果
        return cachedResult, true, nil
    }
    
    if err != redis.Nil {
        return "", false, err
    }
    
    // 未執行過
    return "", false, nil
}

func (m *IdempotencyMiddleware) CacheResult(ctx context.Context, userID, requestID, result string) error {
    key := fmt.Sprintf("idempotency:%s:%s", userID, requestID)
    
    // 快取結果，TTL 5 分鐘
    return m.redis.Set(ctx, key, result, 5*time.Minute).Err()
}
```

#### RPC 處理器整合

```go
// internal/modules/chapter/chapter_service.go
func RpcReadChapter(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    
    var req ReadChapterRequest
    json.Unmarshal([]byte(payload), &req)
    
    // 冪等性檢查
    cachedResult, isDuplicate, err := idempotencyMiddleware.CheckIdempotency(ctx, userID, req.RequestID)
    if err != nil {
        return "", err
    }
    
    if isDuplicate {
        logger.Info("Duplicate request detected, returning cached result", "request_id", req.RequestID)
        return cachedResult, nil
    }
    
    // 執行正常邏輯
    result, err := processReadChapter(ctx, userID, req.ChapterNumber)
    if err != nil {
        return "", err
    }
    
    // 快取結果
    resultJSON, _ := json.Marshal(result)
    idempotencyMiddleware.CacheResult(ctx, userID, req.RequestID, string(resultJSON))
    
    return string(resultJSON), nil
}
```

### 需要冪等性保護的 RPC

| RPC 名稱 | 副作用 | 冪等性保護 |
|---------|-------|----------|
| `ReadChapter` | 扣除靈氣、增加進度 | ✅ 必須 |
| `AttemptBreakthrough` | 更新境界、重置進度 | ✅ 必須 |
| `RestoreQi` | 增加靈氣、記錄交易 | ✅ 必須 |
| `CreateDestiny` | 創建命格 | ✅ 必須 |
| `VerifySubscription` | 更新訂閱狀態 | ✅ 必須 |
| `GetPlayerData` | 僅查詢 | ❌ 不需要 |

### Redis Key 設計

```
Key: idempotency:{user_id}:{request_id}
Value: JSON 格式的 RPC 回應
TTL: 5 分鐘
```

### 錯誤處理

```go
// 如果請求時間戳過舊（超過 5 分鐘），拒絕請求
if time.Now().Unix() - req.Timestamp > 300 {
    return "", errors.New("request expired")
}

// 如果 request_id 格式不正確，拒絕請求
if !isValidUUID(req.RequestID) {
    return "", errors.New("invalid request_id format")
}
```



## 資料保存策略補充說明

### 永久保存的資料

以下資料**永久保存**，不會被自動清理：

1. **玩家基本資料** (`players` collection)
   - 姓名、性別、生日、命格
   - 創建時間、最後登入時間

2. **命格資料** (`destinies` collection)
   - 五行屬性、靈根類型
   - AI 生成的命格描述和出身故事

3. **玩家章節** (`chapters` collection)
   - 所有玩家已閱讀的章節內容
   - 包含普通章節、突破章節、悟道章節

4. **心印閃卡** (`heart_seal_cards` collection - 新增)
   - 每次突破生成的閃卡圖像 URL
   - 境界名稱、突破日期

5. **修煉歷程** (`cultivation_history` collection - 新增)
   - 境界變更記錄
   - 突破成功/失敗記錄
   - 重要里程碑

6. **交易記錄** (PostgreSQL `transactions` table)
   - 所有購買和訂閱記錄
   - 用於財務審計和退款處理

### 可清理的資料

以下資料可以定期清理：

1. **AI 生成快取** (Redis `ai_cache:*`)
   - 未綁定到玩家的 AI 生成結果
   - 超過 30 天未使用的快取

2. **臨時 Session 資料** (Redis)
   - 修煉狀態快取（TTL 1 小時）
   - VIP 狀態快取（TTL 1 小時）

3. **不活躍玩家的 Redis 快取**
   - 30 天未登入玩家的快取資料
   - MongoDB 資料保留，僅清理 Redis

### 資料清理實作

```go
// pkg/database/cleanup_service.go
func (s *CleanupService) CleanupInactiveCache() error {
    thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
    
    // 找出不活躍玩家
    inactiveUsers, err := s.db.Collection("players").Find(
        context.Background(),
        bson.M{"last_login": bson.M{"$lt": thirtyDaysAgo}},
    )
    
    // 僅清理 Redis 快取，保留 MongoDB 資料
    for _, user := range inactiveUsers {
        s.redis.Del(context.Background(), 
            fmt.Sprintf("cultivation:%s", user.UserID),
            fmt.Sprintf("vip:%s", user.UserID),
        )
    }
    
    return nil
}

func (s *CleanupService) CleanupUnusedAICache() error {
    // 清理超過 30 天未使用的 AI 快取
    // 使用 Redis SCAN 遍歷 ai_cache:* keys
    // 檢查最後存取時間，刪除過期的
    
    return nil
}
```

### 資料備份策略

```go
// 每日備份關鍵資料
func (s *BackupService) DailyBackup() error {
    collections := []string{
        "players",
        "destinies", 
        "chapters",
        "heart_seal_cards",
        "cultivation_history",
    }
    
    for _, collection := range collections {
        // 匯出至 S3 或其他備份儲存
        s.ExportCollection(collection)
    }
    
    return nil
}
```



## 多國語言 AI Prompt 策略

### AI 生成內容的語言處理

AI 生成的內容（命格描述、章節內容、悟道章節）需要根據玩家的語言設定使用對應語言的 prompt。

### Prompt 模板多語言化

```json
{
  "templates": [
    {
      "id": "destiny_description",
      "version": "v1.2",
      "prompts": {
        "zh_tw": "你是一位修仙世界的命理大師。根據以下資訊生成命格描述：\n姓名：{name}\n性別：{gender}\n五行屬性：{elements}\n靈根：{spiritual_root}\n\n請用詩意、神秘的語氣描述此人的修仙宿命，字數 200-300 字。",
        "zh_cn": "你是一位修仙世界的命理大师。根据以下信息生成命格描述：\n姓名：{name}\n性别：{gender}\n五行属性：{elements}\n灵根：{spiritual_root}\n\n请用诗意、神秘的语气描述此人的修仙宿命，字数 200-300 字。",
        "en": "You are a fortune teller in a cultivation world. Generate a destiny description based on:\nName: {name}\nGender: {gender}\nFive Elements: {elements}\nSpiritual Root: {spiritual_root}\n\nDescribe this person's cultivation destiny in a poetic and mysterious tone, 200-300 words."
      },
      "parameters": ["name", "gender", "elements", "spiritual_root"]
    },
    {
      "id": "chapter_content",
      "version": "v1.3",
      "prompts": {
        "zh_tw": "你是一位修仙小說作家。根據以下資訊生成章節內容：\n主角：{name}\n當前境界：{realm}\n章節編號：{chapter_number}\n命格特徵：{destiny_traits}\n\n請生成一個修煉故事章節，包含修煉場景、心境描寫和境界感悟，字數 500-800 字。保持與前文連貫。",
        "zh_cn": "你是一位修仙小说作家。根据以下信息生成章节内容：\n主角：{name}\n当前境界：{realm}\n章节编号：{chapter_number}\n命格特征：{destiny_traits}\n\n请生成一个修炼故事章节，包含修炼场景、心境描写和境界感悟，字数 500-800 字。保持与前文连贯。",
        "en": "You are a cultivation novel writer. Generate chapter content based on:\nProtagonist: {name}\nCurrent Realm: {realm}\nChapter Number: {chapter_number}\nDestiny Traits: {destiny_traits}\n\nGenerate a cultivation story chapter with cultivation scenes, mental state descriptions, and realm insights, 500-800 words. Maintain continuity with previous chapters."
      },
      "parameters": ["name", "realm", "chapter_number", "destiny_traits"]
    }
  ]
}
```

### Prompt Manager 多語言支援

```go
// pkg/ai/prompt_manager.go
func (m *PromptManager) GetPrompt(templateID, language string, params map[string]string) (string, string) {
    template := m.templates[templateID]
    
    // 根據語言選擇對應的 prompt
    promptTemplate, exists := template.Prompts[language]
    if !exists {
        // 預設使用英文
        promptTemplate = template.Prompts["en"]
    }
    
    // 替換參數
    prompt := promptTemplate
    for key, value := range params {
        prompt = strings.ReplaceAll(prompt, "{"+key+"}", value)
    }
    
    // 計算 hash（包含語言和版本）
    hashInput := fmt.Sprintf("%s:%s:%s", prompt, language, template.Version)
    hash := sha256.Sum256([]byte(hashInput))
    promptHash := hex.EncodeToString(hash[:])
    
    return prompt, promptHash
}
```

### AI 快取語言隔離

```
Key: ai_cache:{prompt_hash}:{language}:{model_version}
Value: {
  "content": "生成的內容",
  "language": "zh_tw",
  "tokens": 600,
  "cost": 0.0012,
  "created_at": "2025-11-12T10:00:00Z",
  "version": "v1.2"
}
```

### 語言切換處理

當玩家切換語言時：

1. **已生成的章節內容**
   - 保留原語言版本
   - 不重新生成（成本考量）
   - 顯示語言切換提示：「此章節為 {原語言} 生成，切換語言後新章節將使用 {新語言}」

2. **未來章節**
   - 使用新語言的 prompt 生成

3. **UI 文字**
   - 立即切換至新語言（Unity Localization）

### 實作範例

```go
// internal/modules/chapter/chapter_service.go
func (s *ChapterService) GenerateChapter(userID string, chapterNumber int, language string) (string, error) {
    player := s.GetPlayer(userID)
    
    // 構建 prompt 參數
    promptParams := map[string]string{
        "name": player.Name,
        "realm": player.CurrentRealm,
        "chapter_number": strconv.Itoa(chapterNumber),
        "destiny_traits": player.DestinyTraits,
    }
    
    // 獲取對應語言的 prompt
    prompt, hash := s.promptManager.GetPrompt("chapter_content", language, promptParams)
    
    // 檢查快取（包含語言）
    cacheKey := fmt.Sprintf("ai_cache:%s:%s:v1.3", hash, language)
    cached, err := s.redis.Get(context.Background(), cacheKey).Result()
    if err == nil {
        return cached, nil
    }
    
    // 呼叫 AI 生成
    content, err := s.aiGateway.GenerateText(prompt, "gpt-4-mini")
    if err != nil {
        return "", err
    }
    
    // 快取結果
    s.redis.Set(context.Background(), cacheKey, content, 0)
    
    return content, nil
}
```



## API Rate Limit 與重試防護

### 全局限流策略

#### 每用戶限流規則

```go
// pkg/middleware/rate_limiter.go
type RateLimiter struct {
    redis *redis.Client
}

func (r *RateLimiter) CheckRateLimit(ctx context.Context, userID string) error {
    key := fmt.Sprintf("rate_limit:%s", userID)
    
    // 使用 Redis INCR + EXPIRE 實現滑動窗口
    count, err := r.redis.Incr(ctx, key).Result()
    if err != nil {
        return err
    }
    
    // 第一次請求時設定過期時間
    if count == 1 {
        r.redis.Expire(ctx, key, time.Minute)
    }
    
    // 每分鐘最多 60 次請求
    if count > 60 {
        return errors.New("rate limit exceeded: 60 requests per minute")
    }
    
    return nil
}

// 不同 RPC 的限流規則
var rateLimitRules = map[string]int{
    "ReadChapter": 10,        // 每分鐘最多 10 章
    "AttemptBreakthrough": 5, // 每分鐘最多 5 次突破嘗試
    "RestoreQi": 20,          // 每分鐘最多 20 次恢復
    "CreateDestiny": 1,       // 每分鐘最多 1 次創建命格
}

func (r *RateLimiter) CheckRPCRateLimit(ctx context.Context, userID, rpcName string) error {
    limit, exists := rateLimitRules[rpcName]
    if !exists {
        limit = 60 // 預設限制
    }
    
    key := fmt.Sprintf("rate_limit:%s:%s", userID, rpcName)
    count, _ := r.redis.Incr(ctx, key).Result()
    
    if count == 1 {
        r.redis.Expire(ctx, key, time.Minute)
    }
    
    if count > int64(limit) {
        return fmt.Errorf("rate limit exceeded for %s: %d requests per minute", rpcName, limit)
    }
    
    return nil
}
```

#### Nakama RPC 整合

```go
// 在所有 RPC 處理器中加入限流檢查
func RpcReadChapter(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    
    // 限流檢查
    if err := rateLimiter.CheckRPCRateLimit(ctx, userID, "ReadChapter"); err != nil {
        logger.Warn("Rate limit exceeded", "user_id", userID, "rpc", "ReadChapter")
        return "", err
    }
    
    // 正常處理邏輯
    // ...
}
```

#### 異常行為偵測與封鎖

```go
// pkg/security/abuse_detector.go
type AbuseDetector struct {
    redis *redis.Client
}

func (d *AbuseDetector) CheckAbusePattern(ctx context.Context, userID string) error {
    // 檢查是否在黑名單中
    isBlocked, _ := d.redis.Get(ctx, fmt.Sprintf("blocked:%s", userID)).Result()
    if isBlocked == "true" {
        return errors.New("user is blocked due to abuse")
    }
    
    // 檢查短時間內的異常行為
    violations := d.GetViolationCount(ctx, userID)
    if violations > 5 {
        // 封鎖 24 小時
        d.redis.Set(ctx, fmt.Sprintf("blocked:%s", userID), "true", 24*time.Hour)
        
        // 記錄封鎖事件
        d.LogBlockEvent(userID, violations)
        
        return errors.New("user blocked due to repeated violations")
    }
    
    return nil
}
```

### 伺服器端重試策略

```go
// pkg/retry/retry_policy.go
type RetryPolicy struct {
    MaxRetries int
    BaseDelay  time.Duration
}

func (p *RetryPolicy) ExecuteWithRetry(fn func() error) error {
    var lastErr error
    
    for i := 0; i <= p.MaxRetries; i++ {
        err := fn()
        if err == nil {
            return nil
        }
        
        lastErr = err
        
        // 判斷是否應該重試
        if !isRetryableError(err) {
            return err
        }
        
        if i < p.MaxRetries {
            // 指數退避
            delay := p.BaseDelay * time.Duration(1<<uint(i))
            time.Sleep(delay)
        }
    }
    
    return lastErr
}

func isRetryableError(err error) bool {
    // 網路錯誤、超時錯誤可重試
    // 業務邏輯錯誤（如靈氣不足）不重試
    return strings.Contains(err.Error(), "timeout") ||
           strings.Contains(err.Error(), "connection")
}
```

## AI 輸出一致性與版本鎖

### 章節版本控制

#### 章節資料結構更新

```json
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "chapter_id": "uuid",
  "chapter_number": 1,
  "title": "初入修仙界",
  "content": "AI 生成的章節內容...",
  "chapter_type": "normal|breakthrough|enlightenment",
  "realm_at_creation": "qi_refining_early",
  "ai_metadata": {
    "model": "gpt-4-mini",
    "model_version": "2024-11",
    "prompt_version": "v1.3",
    "prompt_hash": "sha256_hash",
    "temperature": 0.8,
    "max_tokens": 800
  },
  "created_at": "2025-11-12T10:00:00Z",
  "read_at": "2025-11-12T10:05:00Z",
  "is_locked": true
}
```

#### 版本鎖機制

```go
// internal/modules/chapter/version_lock.go
type ChapterVersionLock struct {
    db *mongo.Database
}

func (l *ChapterVersionLock) GetOrCreateChapter(userID string, chapterNumber int) (*Chapter, error) {
    // 檢查章節是否已存在
    existing := l.FindChapter(userID, chapterNumber)
    if existing != nil {
        // 已存在，返回鎖定版本
        return existing, nil
    }
    
    // 不存在，生成新章節
    player := l.GetPlayer(userID)
    
    // 使用當前的 prompt 版本和模型
    aiMetadata := AIMetadata{
        Model:         "gpt-4-mini",
        ModelVersion:  "2024-11",
        PromptVersion: "v1.3",
        Temperature:   0.8,
        MaxTokens:     800,
    }
    
    content, promptHash := l.GenerateChapter(player, chapterNumber, aiMetadata)
    
    chapter := &Chapter{
        UserID:        userID,
        ChapterNumber: chapterNumber,
        Content:       content,
        AIMetadata:    aiMetadata,
        IsLocked:      true, // 一旦生成即鎖定
        CreatedAt:     time.Now(),
    }
    
    l.SaveChapter(chapter)
    
    return chapter, nil
}

func (l *ChapterVersionLock) RegenerateChapter(userID string, chapterNumber int) error {
    // 僅允許管理員或特殊情況下重新生成
    chapter := l.FindChapter(userID, chapterNumber)
    if chapter == nil {
        return errors.New("chapter not found")
    }
    
    if chapter.IsLocked {
        return errors.New("chapter is locked, cannot regenerate")
    }
    
    // 重新生成邏輯
    // ...
    
    return nil
}
```

#### 模型版本追蹤

```go
// pkg/ai/model_registry.go
type ModelRegistry struct {
    models map[string]ModelInfo
}

type ModelInfo struct {
    Name           string
    Version        string
    CostPerToken   float64
    MaxTokens      int
    IsDeprecated   bool
    ReplacedBy     string
}

func (r *ModelRegistry) GetCurrentModel(modelName string) ModelInfo {
    model, exists := r.models[modelName]
    if !exists || model.IsDeprecated {
        // 返回替代模型
        return r.models[model.ReplacedBy]
    }
    return model
}

// 確保使用相同版本重新生成
func (r *ModelRegistry) GetModelByVersion(modelName, version string) (ModelInfo, error) {
    key := fmt.Sprintf("%s:%s", modelName, version)
    model, exists := r.models[key]
    if !exists {
        return ModelInfo{}, errors.New("model version not found")
    }
    return model, nil
}
```

### 故事連貫性保證

```go
// 生成新章節時，傳入前幾章的摘要
func (s *ChapterService) GenerateChapterWithContext(userID string, chapterNumber int) (string, error) {
    // 獲取前 3 章的內容摘要
    previousChapters := s.GetPreviousChapters(userID, chapterNumber, 3)
    
    contextSummary := s.SummarizePreviousChapters(previousChapters)
    
    // 在 prompt 中加入上下文
    promptParams := map[string]string{
        "name": player.Name,
        "realm": player.CurrentRealm,
        "chapter_number": strconv.Itoa(chapterNumber),
        "previous_context": contextSummary, // 前文摘要
    }
    
    // 生成章節
    // ...
}
```



## AI 成本控制與模型配額平衡

### 每用戶每日 Token 上限

#### Token 使用追蹤

```go
// pkg/ai/token_quota.go
type TokenQuotaManager struct {
    redis *redis.Client
}

func (m *TokenQuotaManager) CheckQuota(ctx context.Context, userID string, estimatedTokens int) error {
    key := fmt.Sprintf("token_quota:%s:%s", userID, time.Now().Format("2006-01-02"))
    
    // 獲取今日已使用的 token 數
    usedTokens, _ := m.redis.Get(ctx, key).Int()
    
    // 一般用戶每日上限：10,000 tokens（約 10-15 章）
    // VIP 用戶無上限
    isVIP := m.CheckVIPStatus(userID)
    dailyLimit := 10000
    
    if isVIP {
        return nil // VIP 無限制
    }
    
    if usedTokens+estimatedTokens > dailyLimit {
        return fmt.Errorf("daily token quota exceeded: %d/%d", usedTokens, dailyLimit)
    }
    
    return nil
}

func (m *TokenQuotaManager) RecordUsage(ctx context.Context, userID string, tokensUsed int) error {
    key := fmt.Sprintf("token_quota:%s:%s", userID, time.Now().Format("2006-01-02"))
    
    // 累加使用量
    m.redis.IncrBy(ctx, key, int64(tokensUsed))
    
    // 設定過期時間（保留 7 天用於統計）
    m.redis.Expire(ctx, key, 7*24*time.Hour)
    
    return nil
}

func (m *TokenQuotaManager) GetRemainingQuota(ctx context.Context, userID string) int {
    key := fmt.Sprintf("token_quota:%s:%s", userID, time.Now().Format("2006-01-02"))
    usedTokens, _ := m.redis.Get(ctx, key).Int()
    
    isVIP := m.CheckVIPStatus(userID)
    if isVIP {
        return -1 // 表示無限制
    }
    
    return 10000 - usedTokens
}
```

### 高負載時模型降級策略

#### 模型選擇策略

```go
// pkg/ai/model_selector.go
type ModelSelector struct {
    redis *redis.Client
}

type ModelTier struct {
    Name         string
    CostPerToken float64
    Quality      int // 1-5，5 最高
}

var modelTiers = []ModelTier{
    {Name: "gpt-4", CostPerToken: 0.00003, Quality: 5},
    {Name: "gpt-4-mini", CostPerToken: 0.000002, Quality: 4},
    {Name: "claude-haiku", CostPerToken: 0.000001, Quality: 3},
}

func (s *ModelSelector) SelectModel(ctx context.Context, contentType string) string {
    // 檢查當前系統負載
    currentLoad := s.GetSystemLoad(ctx)
    
    // 檢查今日總成本
    dailyCost := s.GetDailyCost(ctx)
    dailyBudget := 100.0 // 每日預算 $100
    
    // 根據負載和成本選擇模型
    if currentLoad > 0.8 || dailyCost > dailyBudget*0.9 {
        // 高負載或接近預算上限，使用低成本模型
        return "claude-haiku"
    } else if currentLoad > 0.5 || dailyCost > dailyBudget*0.7 {
        // 中等負載，使用中等成本模型
        return "gpt-4-mini"
    } else {
        // 低負載，使用高品質模型
        if contentType == "breakthrough" || contentType == "enlightenment" {
            return "gpt-4" // 重要內容使用最好的模型
        }
        return "gpt-4-mini"
    }
}

func (s *ModelSelector) GetSystemLoad(ctx context.Context) float64 {
    // 從 Redis 獲取當前 RPS（每秒請求數）
    key := "system_load:rps"
    rps, _ := s.redis.Get(ctx, key).Float64()
    
    // 假設系統容量為 100 RPS
    maxRPS := 100.0
    return rps / maxRPS
}

func (s *ModelSelector) GetDailyCost(ctx context.Context) float64 {
    key := fmt.Sprintf("daily_cost:%s", time.Now().Format("2006-01-02"))
    cost, _ := s.redis.Get(ctx, key).Float64()
    return cost
}
```

#### 動態調整策略

```go
// 在 AI Gateway 中整合模型選擇
func (g *AIGateway) GenerateText(prompt, preferredModel string) (string, error) {
    // 動態選擇模型
    selectedModel := g.modelSelector.SelectModel(context.Background(), "chapter")
    
    // 如果選擇的模型與偏好不同，記錄日誌
    if selectedModel != preferredModel {
        g.logger.Info("Model downgraded due to load/cost", 
            "preferred", preferredModel, 
            "selected", selectedModel)
    }
    
    // 使用選擇的模型生成
    result, tokens, err := g.callAI(selectedModel, prompt)
    if err != nil {
        return "", err
    }
    
    // 記錄成本
    cost := g.calculateCost(selectedModel, tokens)
    g.recordCost(cost)
    
    return result, nil
}
```

### 成本預警機制

```go
// pkg/ai/cost_monitor.go
type CostMonitor struct {
    redis *redis.Client
}

func (m *CostMonitor) CheckBudgetAlert(ctx context.Context) error {
    dailyCost := m.GetDailyCost(ctx)
    dailyBudget := 100.0
    
    if dailyCost > dailyBudget*0.9 {
        // 發送警報
        m.SendAlert("Daily AI cost exceeds 90% of budget", map[string]interface{}{
            "current_cost": dailyCost,
            "budget": dailyBudget,
            "percentage": (dailyCost / dailyBudget) * 100,
        })
    }
    
    if dailyCost > dailyBudget {
        // 超過預算，暫停非必要的 AI 生成
        return errors.New("daily budget exceeded, AI generation paused")
    }
    
    return nil
}

func (m *CostMonitor) SendAlert(message string, data map[string]interface{}) {
    // 發送至 Slack / Email / Discord
    // ...
}
```

## 玩家進度回溯系統

### 每日快照機制

#### 快照資料結構

```json
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "snapshot_date": "2025-11-12",
  "snapshot_data": {
    "cultivation": {
      "current_qi": 35,
      "current_realm": "qi_refining_early",
      "realm_progress": 45.5
    },
    "chapters_read": 15,
    "last_chapter": 15,
    "breakthrough_count": 2,
    "vip_status": false
  },
  "created_at": "2025-11-12T00:00:00Z"
}
```

#### 快照生成服務

```go
// pkg/snapshot/snapshot_service.go
type SnapshotService struct {
    db *mongo.Database
}

func (s *SnapshotService) CreateDailySnapshot(userID string) error {
    // 獲取玩家當前狀態
    player := s.GetPlayer(userID)
    cultivation := s.GetCultivation(userID)
    chapters := s.GetChapterCount(userID)
    
    snapshot := Snapshot{
        UserID:       userID,
        SnapshotDate: time.Now().Format("2006-01-02"),
        SnapshotData: SnapshotData{
            Cultivation: cultivation,
            ChaptersRead: chapters,
            LastChapter: player.LastChapter,
            BreakthroughCount: player.BreakthroughCount,
            VIPStatus: player.IsVIP,
        },
        CreatedAt: time.Now(),
    }
    
    // 儲存快照
    _, err := s.db.Collection("snapshots").InsertOne(context.Background(), snapshot)
    return err
}

// 每日凌晨自動執行
func (s *SnapshotService) CreateAllSnapshots() error {
    // 獲取所有活躍玩家
    players := s.GetActivePlayers()
    
    for _, player := range players {
        s.CreateDailySnapshot(player.UserID)
    }
    
    return nil
}
```

#### 進度回溯功能

```go
// internal/modules/player/restore_service.go
type RestoreService struct {
    db *mongo.Database
}

func (r *RestoreService) GetAvailableSnapshots(userID string) ([]Snapshot, error) {
    // 獲取最近 3 天的快照
    threeDaysAgo := time.Now().AddDate(0, 0, -3)
    
    snapshots, err := r.db.Collection("snapshots").Find(
        context.Background(),
        bson.M{
            "user_id": userID,
            "created_at": bson.M{"$gte": threeDaysAgo},
        },
    )
    
    return snapshots, err
}

func (r *RestoreService) RestoreToSnapshot(userID, snapshotID string) error {
    // 獲取快照
    snapshot := r.GetSnapshot(snapshotID)
    if snapshot.UserID != userID {
        return errors.New("unauthorized")
    }
    
    // 備份當前狀態（以防誤操作）
    r.CreateBackup(userID)
    
    // 恢復快照資料
    r.UpdateCultivation(userID, snapshot.SnapshotData.Cultivation)
    
    // 記錄恢復操作
    r.LogRestoreEvent(userID, snapshotID)
    
    return nil
}
```

#### 前端回溯介面

```csharp
// UI/Views/ProgressRestoreView.cs
public class ProgressRestoreView : UIView
{
    public async UniTask ShowAvailableSnapshots()
    {
        var snapshots = await NetworkManager.Instance.RpcAsync<List<Snapshot>>("GetSnapshots", null);
        
        foreach (var snapshot in snapshots)
        {
            // 顯示快照列表
            // 日期、境界、章節數等資訊
        }
    }
    
    public async UniTask RestoreSnapshot(string snapshotID)
    {
        // 顯示確認對話框
        var confirmed = await ShowConfirmDialog("確定要回溯至此進度嗎？當前進度將被覆蓋。");
        
        if (confirmed)
        {
            var result = await NetworkManager.Instance.RpcAsync<RestoreResult>("RestoreProgress", new { snapshot_id = snapshotID });
            
            if (result.success)
            {
                ShowMessage("進度已回溯成功");
                ReloadPlayerData();
            }
        }
    }
}
```



## 故事弧線進度追蹤機制

### 資料結構擴展

#### Players Collection 新增欄位

```json
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "name": "玩家姓名",
  "story_progress": {
    "current_arc_id": "arc_1_awakening",
    "arc_progress": 0.45,
    "completed_arcs": ["arc_0_origin"],
    "key_events_completed": ["first_qi_sense", "meet_mentor"],
    "narrative_context": {
      "mentor_name": "雲霄真人",
      "sect_name": "青雲宗",
      "rival_name": "李天行"
    }
  },
  "last_chapter_arc": "arc_1_awakening",
  "created_at": "2025-11-12T00:00:00Z"
}
```

#### Cultivation History 新增弧線記錄

```json
{
  "_id": "ObjectId",
  "user_id": "nakama_user_id",
  "event_type": "arc_completed|key_event|chapter_read",
  "arc_id": "arc_1_awakening",
  "event_data": {
    "arc_name": "覺醒之路",
    "chapters_in_arc": 10,
    "completion_date": "2025-11-15T10:00:00Z"
  },
  "created_at": "2025-11-15T10:00:00Z"
}
```

### 弧線進度追蹤服務

```go
// internal/modules/story/arc_tracker.go
type ArcTracker struct {
    db     *mongo.Database
    config *StoryArcConfig
}

func (t *ArcTracker) GetCurrentArc(userID string) (*StoryArc, error) {
    player := t.GetPlayer(userID)
    
    arcID := player.StoryProgress.CurrentArcID
    arc := t.config.GetArcByID(arcID)
    
    return arc, nil
}

func (t *ArcTracker) UpdateArcProgress(userID string, chapterNumber int) error {
    player := t.GetPlayer(userID)
    currentArc := t.GetCurrentArc(userID)
    
    // 計算弧線進度
    arcProgress := float64(chapterNumber-currentArc.ChapterRange[0]) / 
                   float64(currentArc.ChapterRange[1]-currentArc.ChapterRange[0])
    
    // 更新進度
    player.StoryProgress.ArcProgress = arcProgress
    
    // 檢查是否完成當前弧線
    if chapterNumber >= currentArc.ChapterRange[1] {
        t.CompleteArc(userID, currentArc.ID)
        t.StartNextArc(userID)
    }
    
    // 檢查關鍵事件
    t.CheckKeyEvents(userID, chapterNumber, currentArc)
    
    t.SavePlayer(player)
    
    return nil
}

func (t *ArcTracker) CompleteArc(userID, arcID string) error {
    player := t.GetPlayer(userID)
    
    // 加入已完成弧線列表
    player.StoryProgress.CompletedArcs = append(
        player.StoryProgress.CompletedArcs, 
        arcID,
    )
    
    // 記錄完成事件
    t.LogArcCompletion(userID, arcID)
    
    // Firebase Analytics
    t.analytics.LogEvent("arc_completed", map[string]interface{}{
        "arc_id": arcID,
        "chapters_read": player.ChaptersRead,
    })
    
    return nil
}

func (t *ArcTracker) CheckKeyEvents(userID string, chapterNumber int, arc *StoryArc) {
    for _, keyEvent := range arc.KeyEvents {
        if keyEvent.Chapter == chapterNumber {
            t.TriggerKeyEvent(userID, keyEvent)
        }
    }
}

func (t *ArcTracker) TriggerKeyEvent(userID string, event KeyEvent) {
    player := t.GetPlayer(userID)
    
    // 記錄關鍵事件
    player.StoryProgress.KeyEventsCompleted = append(
        player.StoryProgress.KeyEventsCompleted,
        event.ID,
    )
    
    // 更新敘事上下文（如師父名字、宗門名字等）
    if event.UpdatesContext != nil {
        for key, value := range event.UpdatesContext {
            player.StoryProgress.NarrativeContext[key] = value
        }
    }
    
    t.SavePlayer(player)
    
    // 顯示特殊動畫或提示
    t.SendKeyEventNotification(userID, event)
}
```

### AI 生成整合弧線上下文

```go
// internal/modules/chapter/chapter_service.go
func (s *ChapterService) GenerateChapterWithArcContext(userID string, chapterNumber int) (string, error) {
    player := s.GetPlayer(userID)
    arc := s.arcTracker.GetCurrentArc(userID)
    
    // 構建包含弧線上下文的 prompt
    promptParams := map[string]string{
        "name": player.Name,
        "realm": player.CurrentRealm,
        "chapter_number": strconv.Itoa(chapterNumber),
        "arc_theme": arc.Theme,
        "arc_progress": fmt.Sprintf("%.0f%%", player.StoryProgress.ArcProgress*100),
        "mentor_name": player.StoryProgress.NarrativeContext["mentor_name"],
        "sect_name": player.StoryProgress.NarrativeContext["sect_name"],
    }
    
    // 檢查是否有關鍵事件
    keyEvent := arc.GetKeyEventByChapter(chapterNumber)
    if keyEvent != nil {
        promptParams["key_event"] = keyEvent.Description
    }
    
    // 獲取前文摘要
    previousContext := s.GetPreviousChaptersSummary(userID, 3)
    promptParams["previous_context"] = previousContext
    
    // 生成章節
    prompt, hash := s.promptManager.GetPrompt("chapter_with_arc", player.Language, promptParams)
    content, err := s.aiGateway.GenerateText(prompt, "gpt-4-mini")
    
    // 更新弧線進度
    s.arcTracker.UpdateArcProgress(userID, chapterNumber)
    
    return content, err
}
```

### 弧線配置範例（擴展版）

```json
{
  "story_arcs": [
    {
      "id": "arc_1_awakening",
      "name_key": "story_arc.awakening.name",
      "theme": "初入修仙界，感知靈氣，建立修煉基礎",
      "realm_range": ["qi_refining_early", "qi_refining_mid"],
      "chapter_range": [1, 10],
      "key_events": [
        {
          "id": "first_qi_sense",
          "chapter": 3,
          "event": "首次感知靈氣",
          "description": "主角在靜坐中首次感知到天地靈氣的流動",
          "updates_context": null
        },
        {
          "id": "meet_mentor",
          "chapter": 7,
          "event": "遇見引路人",
          "description": "主角遇見一位神秘的修仙前輩，獲得指點",
          "updates_context": {
            "mentor_name": "雲霄真人",
            "sect_name": "青雲宗"
          }
        },
        {
          "id": "first_trial",
          "chapter": 10,
          "event": "第一次心魔考驗",
          "description": "突破前遭遇心魔，需要克服內心的恐懼"
        }
      ],
      "narrative_elements": {
        "mentor_required": true,
        "sect_required": true,
        "rival_optional": true
      }
    }
  ]
}
```

## 測試策略補強

### 冪等性測試

```go
// test/integration/idempotency_test.go
func TestReadChapterIdempotency(t *testing.T) {
    userID := "test_user_123"
    requestID := uuid.New().String()
    
    // 第一次請求
    req1 := ReadChapterRequest{
        RequestID:     requestID,
        ChapterNumber: 1,
    }
    
    resp1, err := client.RpcAsync("ReadChapter", req1)
    assert.NoError(t, err)
    assert.Equal(t, 40, resp1.CultivationState.CurrentQi) // 50 - 10
    
    // 重複相同請求
    resp2, err := client.RpcAsync("ReadChapter", req1)
    assert.NoError(t, err)
    
    // 應該返回相同結果，靈氣不應該再次扣除
    assert.Equal(t, resp1, resp2)
    assert.Equal(t, 40, resp2.CultivationState.CurrentQi)
}
```

### Mock AI Gateway

```go
// test/mocks/ai_gateway_mock.go
type MockAIGateway struct {
    responses map[string]string
    callCount int
}

func (m *MockAIGateway) GenerateText(prompt, model string) (string, error) {
    m.callCount++
    
    // 返回預設的測試內容
    if response, exists := m.responses[prompt]; exists {
        return response, nil
    }
    
    return "Mock chapter content for testing", nil
}

func (m *MockAIGateway) GetCallCount() int {
    return m.callCount
}

// 測試使用
func TestChapterGeneration(t *testing.T) {
    mockAI := &MockAIGateway{
        responses: map[string]string{
            "test_prompt": "Test chapter content",
        },
    }
    
    service := NewChapterService(mockAI)
    content, err := service.GenerateChapter("user_123", 1)
    
    assert.NoError(t, err)
    assert.Equal(t, "Test chapter content", content)
    assert.Equal(t, 1, mockAI.GetCallCount())
}
```

### API 重試測試

```go
// test/integration/retry_test.go
func TestAPIRetryOnTimeout(t *testing.T) {
    // 模擬超時情況
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(2 * time.Second) // 模擬超時
    }))
    defer server.Close()
    
    client := NewClientWithRetry(server.URL, 3, time.Second)
    
    start := time.Now()
    _, err := client.Call("test_endpoint")
    duration := time.Since(start)
    
    // 應該重試 3 次
    assert.Error(t, err)
    assert.True(t, duration >= 6*time.Second) // 3 次重試，每次 2 秒
}
```

## 新手引導 Analytics 追蹤

### Onboarding 事件定義

```csharp
// Infrastructure/Analytics/OnboardingTracker.cs
public class OnboardingTracker
{
    public void TrackDestinyCreated(DestinyData destiny)
    {
        AnalyticsManager.Instance.LogEvent("onboarding_destiny_created", new {
            spiritual_root = destiny.SpiritualRoot,
            dominant_element = GetDominantElement(destiny.FiveElements),
            session_duration = GetSessionDuration()
        });
    }
    
    public void TrackFirstChapterRead(int chapterNumber, float readingTime)
    {
        AnalyticsManager.Instance.LogEvent("onboarding_first_chapter", new {
            chapter_number = chapterNumber,
            reading_time_seconds = readingTime,
            completion_rate = 1.0f
        });
    }
    
    public void TrackOnboardingStep(string stepName, bool completed)
    {
        AnalyticsManager.Instance.LogEvent("onboarding_step", new {
            step_name = stepName,
            completed = completed,
            step_duration = GetStepDuration(stepName)
        });
    }
    
    public void TrackOnboardingCompleted(float totalDuration)
    {
        AnalyticsManager.Instance.LogEvent("onboarding_completed", new {
            total_duration_seconds = totalDuration,
            steps_completed = GetCompletedStepsCount(),
            first_session = true
        });
    }
}
```

### 推播回流機制

#### Firebase Remote Config 設定

```json
{
  "push_notification_config": {
    "day_1_inactive": {
      "enabled": true,
      "title_key": "push.day1.title",
      "body_key": "push.day1.body",
      "delay_hours": 24
    },
    "day_3_inactive": {
      "enabled": true,
      "title_key": "push.day3.title",
      "body_key": "push.day3.body",
      "delay_hours": 72
    },
    "breakthrough_ready": {
      "enabled": true,
      "title_key": "push.breakthrough.title",
      "body_key": "push.breakthrough.body",
      "trigger": "realm_progress_100"
    }
  }
}
```

#### 推播服務

```go
// pkg/notification/push_service.go
type PushService struct {
    fcm *messaging.Client
    db  *mongo.Database
}

func (s *PushService) ScheduleInactiveUserNotification(userID string) error {
    player := s.GetPlayer(userID)
    
    // 檢查最後登入時間
    daysSinceLastLogin := time.Since(player.LastLogin).Hours() / 24
    
    if daysSinceLastLogin >= 1 && daysSinceLastLogin < 2 {
        // 1 天未登入，發送提醒
        s.SendPushNotification(userID, "day_1_inactive")
    } else if daysSinceLastLogin >= 3 {
        // 3 天未登入，發送回流推播
        s.SendPushNotification(userID, "day_3_inactive")
    }
    
    return nil
}

func (s *PushService) SendBreakthroughReadyNotification(userID string) error {
    cultivation := s.GetCultivation(userID)
    
    if cultivation.RealmProgress >= 100 {
        s.SendPushNotification(userID, "breakthrough_ready")
    }
    
    return nil
}

func (s *PushService) SendPushNotification(userID, notificationType string) error {
    // 獲取玩家的 FCM token
    token := s.GetFCMToken(userID)
    
    // 從 Remote Config 獲取推播內容
    config := s.GetNotificationConfig(notificationType)
    
    message := &messaging.Message{
        Token: token,
        Notification: &messaging.Notification{
            Title: s.GetLocalizedString(config.TitleKey),
            Body:  s.GetLocalizedString(config.BodyKey),
        },
        Data: map[string]string{
            "type": notificationType,
            "user_id": userID,
        },
    }
    
    _, err := s.fcm.Send(context.Background(), message)
    
    // 記錄推播事件
    s.LogPushEvent(userID, notificationType, err == nil)
    
    return err
}
```



## ReadChapter 非同步架構設計

### 問題說明

同步等待 AI 生成會導致：
- 客戶端長時間阻塞（AI 生成需要 5-15 秒）
- 網路超時風險
- 使用者體驗不佳

### 解決方案：非同步生成 + 輪詢機制

#### 架構流程

```
Client                    Nakama Server              AI Service
  │                            │                          │
  │ 1. RequestChapter          │                          │
  ├───────────────────────────>│                          │
  │                            │ 2. 檢查快取               │
  │                            │    (Redis)               │
  │                            │                          │
  │                            │ 3. 若無快取，加入佇列      │
  │                            │    返回 task_id          │
  │<───────────────────────────┤                          │
  │ { status: "generating",    │                          │
  │   task_id: "uuid" }        │                          │
  │                            │                          │
  │ 4. 輪詢狀態                 │                          │
  │    GetChapterStatus        │                          │
  ├───────────────────────────>│                          │
  │                            │ 5. 檢查生成狀態           │
  │                            │    (Redis)               │
  │<───────────────────────────┤                          │
  │ { status: "generating" }   │                          │
  │                            │                          │
  │ ... (每 2 秒輪詢一次)        │                          │
  │                            │                          │
  │                            │ 6. 背景 Worker 處理佇列   │
  │                            ├─────────────────────────>│
  │                            │                          │ 7. AI 生成
  │                            │<─────────────────────────┤
  │                            │ 8. 儲存結果至 MongoDB     │
  │                            │    更新 Redis 狀態        │
  │                            │                          │
  │ 9. 再次輪詢                 │                          │
  ├───────────────────────────>│                          │
  │                            │ 10. 返回完成結果          │
  │<───────────────────────────┤                          │
  │ { status: "completed",     │                          │
  │   chapter: {...} }         │                          │
```

### 後端實作

#### 1. RequestChapter RPC（非同步）

```go
// internal/modules/chapter/chapter_service.go
func RpcRequestChapter(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    
    var req RequestChapterRequest
    json.Unmarshal([]byte(payload), &req)
    
    // 1. 檢查章節是否已存在
    existingChapter := chapterService.FindChapter(userID, req.ChapterNumber)
    if existingChapter != nil {
        // 已存在，直接返回
        return json.Marshal(ChapterResponse{
            Status:  "completed",
            Chapter: existingChapter,
        })
    }
    
    // 2. 檢查是否正在生成中
    taskID := chapterService.GetGeneratingTask(userID, req.ChapterNumber)
    if taskID != "" {
        // 正在生成中，返回 task_id
        return json.Marshal(ChapterResponse{
            Status: "generating",
            TaskID: taskID,
        })
    }
    
    // 3. 檢查靈氣是否足夠
    cultivation := cultivationService.GetCultivation(userID)
    if cultivation.CurrentQi < 10 {
        return "", errors.New("insufficient qi")
    }
    
    // 4. ⚠️ 不在此處扣除靈氣！等生成成功後才扣除
    //    避免生成失敗時玩家損失靈氣
    
    // 5. 加入生成佇列
    taskID = uuid.New().String()
    task := GenerationTask{
        TaskID:        taskID,
        UserID:        userID,
        ChapterNumber: req.ChapterNumber,
        Priority:      "normal",
        CreatedAt:     time.Now(),
    }
    
    generationQueue.Enqueue(task)
    
    // 6. 在 Redis 記錄生成狀態
    redis.Set(ctx, 
        fmt.Sprintf("chapter_task:%s", taskID),
        json.Marshal(task),
        30*time.Minute, // 30 分鐘過期
    )
    
    redis.Set(ctx,
        fmt.Sprintf("user_chapter_task:%s:%d", userID, req.ChapterNumber),
        taskID,
        30*time.Minute,
    )
    
    return json.Marshal(ChapterResponse{
        Status: "generating",
        TaskID: taskID,
    })
}
```

#### 2. GetChapterStatus RPC（輪詢）

```go
// internal/modules/chapter/chapter_service.go
func RpcGetChapterStatus(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    
    var req GetChapterStatusRequest
    json.Unmarshal([]byte(payload), &req)
    
    // 從 Redis 獲取任務狀態
    taskData, err := redis.Get(ctx, fmt.Sprintf("chapter_task:%s", req.TaskID)).Result()
    if err != nil {
        return "", errors.New("task not found")
    }
    
    var task GenerationTask
    json.Unmarshal([]byte(taskData), &task)
    
    // 驗證任務所有權
    if task.UserID != userID {
        return "", errors.New("unauthorized")
    }
    
    // 檢查是否已完成
    if task.Status == "completed" {
        // 從 MongoDB 獲取章節內容
        chapter := chapterService.FindChapter(task.UserID, task.ChapterNumber)
        
        // ✅ 關鍵：此時才扣除靈氣和增加進度
        // 檢查是否已經扣除過（使用冪等性機制）
        consumeKey := fmt.Sprintf("chapter_consumed:%s:%d", userID, task.ChapterNumber)
        alreadyConsumed, _ := redis.Get(ctx, consumeKey).Result()
        
        if alreadyConsumed != "true" {
            // 扣除靈氣
            err := cultivationService.ConsumeQi(userID, 10)
            if err != nil {
                // 靈氣不足（可能被其他操作消耗了）
                // 返回錯誤，但章節已生成，可以考慮允許玩家稍後再讀
                return "", errors.New("insufficient qi to claim chapter")
            }
            
            // 增加境界進度
            cultivationService.UpdateProgress(userID, 10)
            
            // 標記已消耗（防止重複扣除）
            redis.Set(ctx, consumeKey, "true", 24*time.Hour)
        }
        
        // 獲取更新後的修煉狀態
        cultivation := cultivationService.GetCultivation(userID)
        
        return json.Marshal(ChapterResponse{
            Status:  "completed",
            Chapter: chapter,
            CultivationState: cultivation,
        })
    } else if task.Status == "failed" {
        // ❌ 生成失敗，不扣除靈氣
        logger.Warn("Chapter generation failed", "user_id", userID, "chapter", task.ChapterNumber, "error", task.Error)
        
        return json.Marshal(ChapterResponse{
            Status: "failed",
            Error:  task.Error,
            Message: "章節生成失敗，靈氣未被扣除，請稍後重試",
        })
    }
    
    // 仍在生成中
    return json.Marshal(ChapterResponse{
        Status:   "generating",
        TaskID:   req.TaskID,
        Progress: task.Progress,
    })
}
```

#### 3. 背景 Worker 處理佇列

```go
// internal/modules/chapter/generation_worker.go
type GenerationWorker struct {
    queue      *GenerationQueue
    aiGateway  *AIGateway
    redis      *redis.Client
    db         *mongo.Database
    maxWorkers int
}

func (w *GenerationWorker) Start() {
    for i := 0; i < w.maxWorkers; i++ {
        go w.processQueue()
    }
}

func (w *GenerationWorker) processQueue() {
    for {
        // 從佇列取出任務
        task := w.queue.Dequeue()
        if task == nil {
            time.Sleep(time.Second)
            continue
        }
        
        // 更新狀態為處理中
        task.Status = "processing"
        w.updateTaskStatus(task)
        
        // 生成章節
        err := w.generateChapter(task)
        if err != nil {
            task.Status = "failed"
            task.Error = err.Error()
            w.updateTaskStatus(task)
            continue
        }
        
        // 標記為完成
        task.Status = "completed"
        w.updateTaskStatus(task)
    }
}

func (w *GenerationWorker) generateChapter(task *GenerationTask) error {
    player := w.getPlayer(task.UserID)
    
    // 構建 prompt
    promptParams := map[string]string{
        "name":           player.Name,
        "realm":          player.CurrentRealm,
        "chapter_number": strconv.Itoa(task.ChapterNumber),
    }
    
    prompt, hash := w.promptManager.GetPrompt("chapter_content", player.Language, promptParams)
    
    // 呼叫 AI 生成（帶重試機制）
    var content string
    var tokens int
    var err error
    
    for retry := 0; retry < 3; retry++ {
        content, tokens, err = w.aiGateway.GenerateText(prompt, "gpt-4-mini")
        if err == nil {
            break
        }
        
        w.logger.Warn("AI generation failed, retrying", "attempt", retry+1, "error", err)
        time.Sleep(time.Second * time.Duration(retry+1))
    }
    
    if err != nil {
        // 3 次重試都失敗
        return fmt.Errorf("AI generation failed after 3 retries: %w", err)
    }
    
    // 內容審查（可選）
    if !w.contentModerator.IsApproved(content) {
        return errors.New("content moderation failed")
    }
    
    // 儲存章節
    chapter := &Chapter{
        UserID:        task.UserID,
        ChapterNumber: task.ChapterNumber,
        Content:       content,
        AIMetadata: AIMetadata{
            Model:         "gpt-4-mini",
            PromptVersion: "v1.3",
            PromptHash:    hash,
            TokensUsed:    tokens,
        },
        CreatedAt: time.Now(),
    }
    
    _, err = w.db.Collection("chapters").InsertOne(context.Background(), chapter)
    if err != nil {
        return fmt.Errorf("failed to save chapter: %w", err)
    }
    
    // ⚠️ 注意：不在此處扣除靈氣或更新進度
    //    這些操作在 GetChapterStatus 中完成
    //    確保玩家只有在成功獲取章節時才消耗資源
    
    return nil
}

func (w *GenerationWorker) updateTaskStatus(task *GenerationTask) {
    taskJSON, _ := json.Marshal(task)
    w.redis.Set(context.Background(),
        fmt.Sprintf("chapter_task:%s", task.TaskID),
        taskJSON,
        30*time.Minute,
    )
}
```

### 前端實作

#### 非同步請求與輪詢

```csharp
// Domain/Chapter/ChapterService.cs
public class ChapterService
{
    private NetworkManager networkManager;
    private const float POLL_INTERVAL = 2f; // 每 2 秒輪詢一次
    
    public async UniTask<Chapter> RequestChapterAsync(int chapterNumber)
    {
        // 1. 發送請求
        var response = await networkManager.RpcAsync<ChapterResponse>("RequestChapter", new {
            chapter_number = chapterNumber
        });
        
        if (response.status == "completed")
        {
            // 已存在，直接返回
            return response.chapter;
        }
        
        if (response.status == "generating")
        {
            // 正在生成，開始輪詢
            return await PollChapterStatusAsync(response.task_id);
        }
        
        throw new Exception("Unexpected response status");
    }
    
    private async UniTask<Chapter> PollChapterStatusAsync(string taskID)
    {
        var maxAttempts = 30; // 最多輪詢 30 次（60 秒）
        var attempts = 0;
        
        while (attempts < maxAttempts)
        {
            await UniTask.Delay(TimeSpan.FromSeconds(POLL_INTERVAL));
            
            var response = await networkManager.RpcAsync<ChapterResponse>("GetChapterStatus", new {
                task_id = taskID
            });
            
            if (response.status == "completed")
            {
                return response.chapter;
            }
            
            if (response.status == "failed")
            {
                throw new Exception($"Chapter generation failed: {response.error}");
            }
            
            // 更新進度（可選）
            OnGenerationProgress?.Invoke(response.progress);
            
            attempts++;
        }
        
        throw new TimeoutException("Chapter generation timeout");
    }
    
    public event Action<float> OnGenerationProgress;
}
```

#### UI 顯示生成進度

```csharp
// UI/Views/ChapterReadingView.cs
public async UniTask ReadNextChapter()
{
    // 顯示載入動畫
    ShowLoadingAnimation("天機推演中...");
    
    try
    {
        // 訂閱進度事件
        chapterService.OnGenerationProgress += UpdateLoadingProgress;
        
        var chapter = await chapterService.RequestChapterAsync(currentChapterNumber + 1);
        
        // 隱藏載入動畫
        HideLoadingAnimation();
        
        // 顯示章節內容
        DisplayChapter(chapter);
    }
    catch (Exception ex)
    {
        HideLoadingAnimation();
        ShowError($"章節生成失敗：{ex.Message}");
    }
    finally
    {
        chapterService.OnGenerationProgress -= UpdateLoadingProgress;
    }
}

private void UpdateLoadingProgress(float progress)
{
    loadingProgressBar.value = progress;
    loadingText.text = $"生成中... {progress:P0}";
}
```

### Redis 資料結構

```
# 任務狀態
Key: chapter_task:{task_id}
Value: {
  "task_id": "uuid",
  "user_id": "nakama_user_id",
  "chapter_number": 5,
  "status": "generating|processing|completed|failed",
  "progress": 0.5,
  "error": null,
  "created_at": "2025-11-12T10:00:00Z"
}
TTL: 30 分鐘

# 使用者章節任務映射
Key: user_chapter_task:{user_id}:{chapter_number}
Value: task_id
TTL: 30 分鐘
```

### 關鍵設計決策

#### 🔑 靈氣扣除時機

**問題**：何時扣除玩家的靈氣？

**錯誤做法**：在 RequestChapter 時立即扣除
- ❌ 如果 AI 生成失敗，玩家損失靈氣但沒有獲得章節
- ❌ 會導致客訴和負面評價

**正確做法**：在 GetChapterStatus 返回成功時才扣除
- ✅ 只有成功生成章節，玩家才消耗靈氣
- ✅ 生成失敗時，玩家可以重試而不損失資源
- ✅ 使用冪等性機制防止重複扣除

#### 🔑 冪等性保證

使用 Redis key `chapter_consumed:{user_id}:{chapter_number}` 確保：
- 即使玩家多次輪詢 GetChapterStatus
- 靈氣只會被扣除一次
- 進度只會增加一次

#### 🔑 錯誤處理

**AI 生成失敗的處理**：
1. Worker 重試 3 次
2. 仍失敗則標記任務為 "failed"
3. GetChapterStatus 返回錯誤訊息
4. **不扣除靈氣**
5. 玩家可以重新發起 RequestChapter

**靈氣不足的處理**：
- RequestChapter 時檢查靈氣（≥10）
- GetChapterStatus 時再次檢查（可能被其他操作消耗）
- 若不足，返回錯誤但保留已生成的章節
- 玩家恢復靈氣後可以再次嘗試獲取

### 優勢

1. **非阻塞**：客戶端不會長時間等待
2. **可擴展**：可以增加 Worker 數量處理更多並發
3. **容錯性**：任務失敗可以重試，不影響其他任務
4. **使用者體驗**：顯示生成進度，提供即時反饋
5. **成本控制**：可以在 Worker 層面控制並發數和優先級
6. **公平性**：只有成功獲得章節才消耗靈氣，避免玩家損失



## 突破系統非同步架構設計

### 問題說明

突破成功後需要生成：
1. **悟道章節**：AI 文字生成（5-15 秒）
2. **心印閃卡**：AI 圖像生成（30-120 秒）

如果同步等待，玩家需要等待 **35-135 秒**，這是不可接受的使用者體驗。

### 解決方案：分離判定與內容生成

#### 架構流程

```
Client                    Nakama Server              AI Service
  │                            │                          │
  │ 1. AttemptBreakthrough     │                          │
  ├───────────────────────────>│                          │
  │                            │ 2. 驗證進度               │
  │                            │ 3. 計算成功率             │
  │                            │ 4. 執行隨機判定           │
  │                            │                          │
  │                            │ 5. 若成功：               │
  │                            │    - 更新境界             │
  │                            │    - 創建生成任務         │
  │                            │    - 立即返回結果         │
  │<───────────────────────────┤                          │
  │ { breakthrough_success: true,                         │
  │   task_id: "uuid",                                    │
  │   status: "generating" }   │                          │
  │                            │                          │
  │ 6. 顯示突破成功動畫         │                          │
  │    (不等待內容生成)         │                          │
  │                            │                          │
  │                            │ 7. 背景 Worker 生成內容   │
  │                            ├─────────────────────────>│
  │                            │                          │ 8. 生成悟道章節
  │                            │<─────────────────────────┤
  │                            │                          │
  │                            ├─────────────────────────>│
  │                            │                          │ 9. 生成心印閃卡
  │                            │<─────────────────────────┤
  │                            │ 10. 儲存結果              │
  │                            │                          │
  │ 11. 輪詢狀態                │                          │
  │     GetBreakthroughStatus  │                          │
  ├───────────────────────────>│                          │
  │<───────────────────────────┤                          │
  │ { status: "completed",     │                          │
  │   enlightenment_chapter,   │                          │
  │   heart_seal_card }        │                          │
```

### 後端實作

#### 1. AttemptBreakthrough RPC（快速返回）

```go
// internal/modules/cultivation/breakthrough_service.go
func RpcAttemptBreakthrough(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    
    var req AttemptBreakthroughRequest
    json.Unmarshal([]byte(payload), &req)
    
    // 1. 驗證境界進度
    cultivation := cultivationService.GetCultivation(userID)
    if cultivation.RealmProgress < 100 {
        return "", errors.New("realm progress not ready for breakthrough")
    }
    
    // 2. 計算成功率
    player := playerService.GetPlayer(userID)
    successRate := breakthroughService.CalculateSuccessRate(
        cultivation.CurrentRealm,
        player.SpiritualRoot,
    )
    
    // 3. 執行隨機判定
    success := rand.Float64() <= successRate
    
    // 4. 記錄突破嘗試
    breakthroughService.RecordAttempt(userID, cultivation.CurrentRealm, success, successRate)
    
    if !success {
        // 失敗：重置進度，立即返回
        cultivation.RealmProgress = 80
        cultivationService.SaveCultivation(cultivation)
        
        // Firebase Analytics
        analytics.LogEvent("breakthrough_fail", map[string]interface{}{
            "realm": cultivation.CurrentRealm,
            "success_rate": successRate,
        })
        
        return json.Marshal(BreakthroughResponse{
            Success:             true,
            BreakthroughSuccess: false,
            CurrentRealm:        cultivation.CurrentRealm,
            RealmProgress:       80,
            Message:             "突破失敗，境界進度重置至 80%",
        })
    }
    
    // 5. 成功：更新境界
    nextRealm := configService.GetNextRealm(cultivation.CurrentRealm)
    cultivation.CurrentRealm = nextRealm.ID
    cultivation.RealmProgress = 0
    cultivation.BreakthroughCount++
    cultivationService.SaveCultivation(cultivation)
    
    // 6. 創建非同步生成任務
    taskID := uuid.New().String()
    task := BreakthroughContentTask{
        TaskID:    taskID,
        UserID:    userID,
        OldRealm:  req.CurrentRealm,
        NewRealm:  nextRealm.ID,
        CreatedAt: time.Now(),
        Status:    "pending",
    }
    
    // 加入生成佇列
    breakthroughQueue.Enqueue(task)
    
    // 在 Redis 記錄任務狀態
    redis.Set(ctx,
        fmt.Sprintf("breakthrough_task:%s", taskID),
        json.Marshal(task),
        30*time.Minute,
    )
    
    // Firebase Analytics
    analytics.LogEvent("breakthrough_success", map[string]interface{}{
        "old_realm": req.CurrentRealm,
        "new_realm": nextRealm.ID,
        "success_rate": successRate,
    })
    
    // 7. 立即返回成功結果（不等待內容生成）
    return json.Marshal(BreakthroughResponse{
        Success:             true,
        BreakthroughSuccess: true,
        NewRealm:            nextRealm.ID,
        TaskID:              taskID,
        Status:              "generating_content",
        Message:             "突破成功！正在生成悟道章節與心印閃卡...",
    })
}
```

#### 2. GetBreakthroughStatus RPC（輪詢）

```go
// internal/modules/cultivation/breakthrough_service.go
func RpcGetBreakthroughStatus(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    
    var req GetBreakthroughStatusRequest
    json.Unmarshal([]byte(payload), &req)
    
    // 從 Redis 獲取任務狀態
    taskData, err := redis.Get(ctx, fmt.Sprintf("breakthrough_task:%s", req.TaskID)).Result()
    if err != nil {
        return "", errors.New("task not found")
    }
    
    var task BreakthroughContentTask
    json.Unmarshal([]byte(taskData), &task)
    
    // 驗證任務所有權
    if task.UserID != userID {
        return "", errors.New("unauthorized")
    }
    
    if task.Status == "completed" {
        // 從 MongoDB 獲取生成的內容
        enlightenmentChapter := chapterService.FindEnlightenmentChapter(userID, task.NewRealm)
        heartSealCard := cardService.FindHeartSealCard(userID, task.NewRealm)
        
        return json.Marshal(BreakthroughStatusResponse{
            Status:               "completed",
            EnlightenmentChapter: enlightenmentChapter,
            HeartSealCard:        heartSealCard,
        })
    } else if task.Status == "failed" {
        return json.Marshal(BreakthroughStatusResponse{
            Status:  "failed",
            Error:   task.Error,
            Message: "內容生成失敗，但突破已成功。您可以稍後在修行歷程中查看。",
        })
    }
    
    // 仍在生成中
    return json.Marshal(BreakthroughStatusResponse{
        Status:      "generating",
        Progress:    task.Progress,
        CurrentStep: task.CurrentStep, // "generating_chapter" | "generating_card"
    })
}
```

#### 3. 背景 Worker 生成內容

```go
// internal/modules/cultivation/breakthrough_worker.go
type BreakthroughWorker struct {
    queue     *BreakthroughQueue
    aiGateway *AIGateway
    redis     *redis.Client
    db        *mongo.Database
}

func (w *BreakthroughWorker) processQueue() {
    for {
        task := w.queue.Dequeue()
        if task == nil {
            time.Sleep(time.Second)
            continue
        }
        
        task.Status = "processing"
        w.updateTaskStatus(task)
        
        // 生成悟道章節
        task.CurrentStep = "generating_chapter"
        task.Progress = 0.1
        w.updateTaskStatus(task)
        
        enlightenmentChapter, err := w.generateEnlightenmentChapter(task)
        if err != nil {
            task.Status = "failed"
            task.Error = err.Error()
            w.updateTaskStatus(task)
            continue
        }
        
        task.Progress = 0.5
        w.updateTaskStatus(task)
        
        // 生成心印閃卡
        task.CurrentStep = "generating_card"
        w.updateTaskStatus(task)
        
        heartSealCard, err := w.generateHeartSealCard(task)
        if err != nil {
            task.Status = "failed"
            task.Error = err.Error()
            w.updateTaskStatus(task)
            continue
        }
        
        task.Progress = 1.0
        task.Status = "completed"
        w.updateTaskStatus(task)
    }
}

func (w *BreakthroughWorker) generateEnlightenmentChapter(task *BreakthroughContentTask) (*Chapter, error) {
    player := w.getPlayer(task.UserID)
    
    // 構建 prompt
    promptParams := map[string]string{
        "name":      player.Name,
        "old_realm": task.OldRealm,
        "new_realm": task.NewRealm,
    }
    
    prompt, hash := w.promptManager.GetPrompt("enlightenment_chapter", player.Language, promptParams)
    
    // 呼叫 AI 生成（使用高品質模型）
    content, tokens, err := w.aiGateway.GenerateText(prompt, "gpt-4")
    if err != nil {
        return nil, err
    }
    
    // 儲存悟道章節
    chapter := &Chapter{
        UserID:      task.UserID,
        ChapterType: "enlightenment",
        Realm:       task.NewRealm,
        Content:     content,
        AIMetadata: AIMetadata{
            Model:         "gpt-4",
            PromptVersion: "v1.0",
            PromptHash:    hash,
            TokensUsed:    tokens,
        },
        CreatedAt: time.Now(),
    }
    
    w.db.Collection("chapters").InsertOne(context.Background(), chapter)
    
    return chapter, nil
}

func (w *BreakthroughWorker) generateHeartSealCard(task *BreakthroughContentTask) (*HeartSealCard, error) {
    player := w.getPlayer(task.UserID)
    realmName := w.configService.GetRealmName(task.NewRealm)
    
    // 構建圖像生成 prompt
    prompt := fmt.Sprintf("A mystical cultivation realm card for %s, Chinese xianxia style, elegant calligraphy, spiritual energy flowing", realmName)
    
    // 呼叫 AI 圖像生成
    imageURL, err := w.aiGateway.GenerateImage(prompt, "stable-diffusion")
    if err != nil {
        return nil, err
    }
    
    // 儲存心印閃卡
    card := &HeartSealCard{
        UserID:    task.UserID,
        Realm:     task.NewRealm,
        RealmName: realmName,
        ImageURL:  imageURL,
        Date:      time.Now().Format("2006-01-02"),
        CreatedAt: time.Now(),
    }
    
    w.db.Collection("heart_seal_cards").InsertOne(context.Background(), card)
    
    return card, nil
}
```

### 前端實作

#### 突破流程

```csharp
// Domain/Cultivation/BreakthroughService.cs
public async UniTask<BreakthroughResult> AttemptBreakthroughAsync()
{
    // 1. 發送突破請求
    var response = await networkManager.RpcAsync<BreakthroughResponse>("AttemptBreakthrough", new {
        current_realm = currentRealm,
        realm_progress = realmProgress,
        request_id = Guid.NewGuid().ToString()
    });
    
    if (!response.breakthrough_success)
    {
        // 失敗：立即返回
        return new BreakthroughResult {
            Success = false,
            Message = response.message
        };
    }
    
    // 2. 成功：播放突破動畫（不等待內容生成）
    await PlayBreakthroughAnimation(response.new_realm);
    
    // 3. 顯示「正在生成悟道內容」提示
    ShowGeneratingNotification();
    
    // 4. 開始輪詢內容生成狀態（背景進行）
    PollBreakthroughContentAsync(response.task_id).Forget();
    
    return new BreakthroughResult {
        Success = true,
        NewRealm = response.new_realm,
        TaskID = response.task_id
    };
}

private async UniTaskVoid PollBreakthroughContentAsync(string taskID)
{
    var maxAttempts = 60; // 最多輪詢 60 次（2 分鐘）
    var attempts = 0;
    
    while (attempts < maxAttempts)
    {
        await UniTask.Delay(TimeSpan.FromSeconds(2));
        
        var response = await networkManager.RpcAsync<BreakthroughStatusResponse>("GetBreakthroughStatus", new {
            task_id = taskID
        });
        
        if (response.status == "completed")
        {
            // 內容生成完成，顯示通知
            ShowContentReadyNotification();
            
            // 儲存內容供玩家查看
            SaveEnlightenmentContent(response.enlightenment_chapter, response.heart_seal_card);
            
            return;
        }
        
        if (response.status == "failed")
        {
            // 生成失敗，但突破已成功
            ShowContentGenerationFailedNotification();
            return;
        }
        
        // 更新進度
        UpdateGenerationProgress(response.progress, response.current_step);
        
        attempts++;
    }
    
    // 超時，但突破已成功
    ShowContentGenerationTimeoutNotification();
}
```

#### UI 流程

```csharp
// UI/Views/BreakthroughView.cs
public async UniTask OnBreakthroughButtonClicked()
{
    // 1. 播放點擊動畫
    breakthroughButton.interactable = false;
    
    // 2. 執行突破
    var result = await breakthroughService.AttemptBreakthroughAsync();
    
    if (!result.Success)
    {
        // 失敗：顯示失敗動畫和訊息
        await PlayFailureAnimation();
        ShowMessage(result.Message);
        breakthroughButton.interactable = true;
        return;
    }
    
    // 3. 成功：播放成功動畫（約 3-5 秒）
    await PlaySuccessAnimation(result.NewRealm);
    
    // 4. 顯示新境界資訊
    ShowNewRealmInfo(result.NewRealm);
    
    // 5. 顯示「悟道內容生成中」提示（非阻塞）
    ShowGeneratingBanner("正在生成悟道章節與心印閃卡，完成後將通知您");
    
    // 6. 玩家可以繼續遊戲，不需要等待
    Close();
}
```

### 關鍵設計決策

#### 🔑 分離判定與內容生成

**判定階段（同步，< 1 秒）**：
- 計算成功率
- 執行隨機判定
- 更新境界
- 立即返回結果

**內容生成階段（非同步，30-120 秒）**：
- 生成悟道章節
- 生成心印閃卡
- 背景進行，不阻塞玩家

#### 🔑 使用者體驗優化

1. **立即反饋**：突破結果在 1 秒內返回
2. **非阻塞**：玩家可以繼續遊戲，不需要等待內容生成
3. **通知機制**：內容生成完成後推送通知
4. **容錯處理**：即使內容生成失敗，突破仍然成功

#### 🔑 內容生成失敗處理

- 突破判定已成功，境界已更新
- 內容生成失敗不影響突破結果
- 玩家可以稍後在「修行歷程」中查看
- 可以提供「重新生成」選項

### 優勢

1. **極佳的使用者體驗**：突破結果立即返回
2. **非阻塞**：玩家不需要等待 AI 生成
3. **容錯性**：內容生成失敗不影響突破
4. **可擴展**：可以增加 Worker 處理更多並發
5. **成本可控**：可以在 Worker 層面控制生成優先級



## 命格系統完整流程設計

### 設計理念：分離「刷」的樂趣與「生成」的成本

**核心原則**
- 玩家需求：刷到一個好命格（Gacha 樂趣）
- 系統需求：只在玩家確認後才支付 AI 成本
- 解決方案：將「刷數據」和「生成故事」分離

### 三階段流程

#### 階段一：【刷命格】（免費、快速、可重複）

**目標**：讓玩家在零成本、零延遲下，刷到滿意的命格數據

**UX 流程**
```
玩家輸入姓名、性別、生日
  ↓
點擊「① 推演命格」
  ↓
< 0.5 秒：顯示五行屬性和靈根
  ↓
不滿意？點擊「② 重新推演」
  ↓
無限次重複，直到滿意
  ↓
點擊「③ 鎖定此命格入道」
```

**前端實作**
```csharp
// UI/Views/DestinyCreationView.cs
public async UniTask OnCalculateDestinyClicked()
{
    var request = new CalculateDestinyRequest {
        name = nameInput.value,
        gender = genderDropdown.value,
        birth_date = birthDatePicker.value
    };
    
    // 快速計算（< 0.5 秒）
    var result = await networkManager.RpcAsync<CalculateDestinyResponse>("CalculateDestiny", request);
    
    // 立即顯示數據
    DisplayDestinyData(result);
    
    // 啟用「重新推演」和「鎖定命格」按鈕
    rerollButton.SetEnabled(true);
    confirmButton.SetEnabled(true);
}

public async UniTask OnRerollClicked()
{
    // 記錄重刷次數
    rerollCount++;
    AnalyticsManager.Instance.LogEvent("destiny_rerolled", new { count = rerollCount });
    
    // 重新計算（免費、快速）
    await OnCalculateDestinyClicked();
}

public async UniTask OnConfirmDestinyClicked()
{
    // 顯示確認對話框
    var confirmed = await ShowConfirmDialog("道友，天命已定，此生無法更改。是否確認以此命格入道？");
    
    if (!confirmed) return;
    
    // 記錄確認事件
    AnalyticsManager.Instance.LogEvent("destiny_confirmed", new {
        spiritual_root = currentDestiny.spiritual_root,
        reroll_count = rerollCount
    });
    
    // 進入階段二
    await GenerateDestinyStoryAsync();
}
```

**後端實作**
```go
// internal/modules/destiny/destiny_service.go
func RpcCalculateDestiny(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    var req CalculateDestinyRequest
    json.Unmarshal([]byte(payload), &req)
    
    // 驗證輸入
    if err := validateInput(req); err != nil {
        return "", err
    }
    
    // 純數學計算五行屬性（基於生辰八字）
    fiveElements := calculateFiveElements(req.BirthDate)
    
    // 判定靈根類型
    spiritualRoot := determineSpiritualRoot(fiveElements)
    
    // 判定品質
    quality := determineQuality(fiveElements)
    
    // 不呼叫 AI，不儲存資料，立即返回
    response := CalculateDestinyResponse{
        Success:             true,
        FiveElements:        fiveElements,
        SpiritualRoot:       spiritualRoot,
        SpiritualRootQuality: quality,
    }
    
    out, _ := json.Marshal(response)
    return string(out), nil
}
```

#### 階段二：【觀天命】（一次性 AI 生成，有等待）

**目標**：在玩家做出承諾後，才執行唯一一次的 AI 故事生成

**UX 流程**
```
玩家點擊「鎖定此命格入道」
  ↓
確認對話框
  ↓
點擊「確認」
  ↓
沉浸式等待動畫（5-10 秒）
「天機推演中，正在為您撰寫出身...」
  ↓
輪詢生成狀態
  ↓
完成：顯示華麗的命格卡片
（數據 + AI 生成的故事）
  ↓
點擊「開始修煉」
```

**前端實作**
```csharp
private async UniTask GenerateDestinyStoryAsync()
{
    // 顯示沉浸式等待動畫
    ShowImmersiveLoading("天機推演中，正在為您撰寫出身...");
    
    // 呼叫 ConfirmDestiny
    var request = new ConfirmDestinyRequest {
        name = currentDestiny.name,
        gender = currentDestiny.gender,
        birth_date = currentDestiny.birth_date,
        five_elements = currentDestiny.five_elements,
        spiritual_root = currentDestiny.spiritual_root
    };
    
    var response = await networkManager.RpcAsync<ConfirmDestinyResponse>("ConfirmDestiny", request);
    
    if (response.status == "generating")
    {
        // 輪詢生成狀態
        var destiny = await PollDestinyStatusAsync(response.task_id);
        
        // 隱藏載入動畫
        HideImmersiveLoading();
        
        // 顯示華麗的命格卡片
        await ShowDestinyCard(destiny);
    }
}

private async UniTask<Destiny> PollDestinyStatusAsync(string taskID)
{
    var maxAttempts = 30;
    var attempts = 0;
    
    while (attempts < maxAttempts)
    {
        await UniTask.Delay(TimeSpan.FromSeconds(2));
        
        var response = await networkManager.RpcAsync<GetDestinyStatusResponse>("GetDestinyStatus", new {
            task_id = taskID
        });
        
        if (response.status == "completed")
        {
            return response.destiny;
        }
        
        if (response.status == "failed")
        {
            throw new Exception("命格故事生成失敗");
        }
        
        attempts++;
    }
    
    throw new TimeoutException("命格故事生成超時");
}
```

**後端實作**
```go
// internal/modules/destiny/destiny_service.go
func RpcConfirmDestiny(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, payload string) (string, error) {
    userID := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
    
    var req ConfirmDestinyRequest
    json.Unmarshal([]byte(payload), &req)
    
    // 1. 驗證數據（重新計算五行，確認一致性）
    calculatedElements := calculateFiveElements(req.BirthDate)
    if !elementsMatch(calculatedElements, req.FiveElements) {
        return "", errors.New("destiny data mismatch, possible tampering")
    }
    
    // 2. 檢查玩家是否已有命格（防止重複創建）
    existingDestiny := destinyService.FindDestiny(userID)
    if existingDestiny != nil {
        return "", errors.New("destiny already exists")
    }
    
    // 3. 創建非同步生成任務
    taskID := uuid.New().String()
    task := DestinyGenerationTask{
        TaskID:        taskID,
        UserID:        userID,
        Name:          req.Name,
        Gender:        req.Gender,
        BirthDate:     req.BirthDate,
        FiveElements:  req.FiveElements,
        SpiritualRoot: req.SpiritualRoot,
        Status:        "pending",
        CreatedAt:     time.Now(),
    }
    
    // 加入佇列
    destinyQueue.Enqueue(task)
    
    // 記錄至 Redis
    redis.Set(ctx,
        fmt.Sprintf("destiny_task:%s", taskID),
        json.Marshal(task),
        30*time.Minute,
    )
    
    // 4. 立即返回（不等待 AI）
    return json.Marshal(ConfirmDestinyResponse{
        Success: true,
        TaskID:  taskID,
        Status:  "generating",
        Message: "天機推演中，正在為您撰寫出身...",
    })
}

// DestinyWorker 背景生成
func (w *DestinyWorker) generateDestinyStory(task *DestinyGenerationTask) error {
    // 構建 prompt
    promptParams := map[string]string{
        "name":           task.Name,
        "gender":         task.Gender,
        "spiritual_root": task.SpiritualRoot,
        "elements":       formatElements(task.FiveElements),
    }
    
    prompt, hash := w.promptManager.GetPrompt("destiny_description", task.Language, promptParams)
    
    // 呼叫 AI 生成
    description, tokens, err := w.aiGateway.GenerateText(prompt, "gpt-4-mini")
    if err != nil {
        return err
    }
    
    // 生成出身故事
    originPrompt, originHash := w.promptManager.GetPrompt("origin_story", task.Language, promptParams)
    originStory, originTokens, err := w.aiGateway.GenerateText(originPrompt, "gpt-4-mini")
    if err != nil {
        return err
    }
    
    // 儲存命格
    destiny := &Destiny{
        UserID:              task.UserID,
        Name:                task.Name,
        Gender:              task.Gender,
        BirthDate:           task.BirthDate,
        FiveElements:        task.FiveElements,
        SpiritualRoot:       task.SpiritualRoot,
        DestinyDescription:  description,
        OriginStory:         originStory,
        CreatedAt:           time.Now(),
    }
    
    w.db.Collection("destinies").InsertOne(context.Background(), destiny)
    
    // 初始化修煉資料
    w.cultivationService.InitializeCultivation(task.UserID)
    
    // 記錄 AI 成本
    w.costTracker.RecordUsage(task.UserID, "destiny", tokens+originTokens)
    
    return nil
}
```

#### 階段三：【入道途】（第二次 AI 生成，無縫銜接）

**目標**：處理第一章的生成延遲，避免玩家連續等待

**UX 流程**
```
玩家點擊「開始修煉」
  ↓
全螢幕過場動畫
「道途開啟...」
「引氣入體...」
「感知天地靈氣...」
  ↓
背景呼叫 RequestChapter(1)
  ↓
輪詢生成狀態，進度條顯示真實進度
  ↓
完成：過場動畫淡出
  ↓
無縫進入 ChapterReadingView
第一章內容已顯示
```

**前端實作**
```csharp
// UI/Views/DestinyCardView.cs
public async UniTask OnStartCultivationClicked()
{
    // 顯示全螢幕過場動畫
    var loadingView = await UIManager.Instance.OpenAsync<CultivationLoadingView>();
    
    try
    {
        // 背景請求第一章
        loadingView.SetMessage("道途開啟...");
        
        chapterService.OnGenerationProgress += loadingView.UpdateProgress;
        
        var firstChapter = await chapterService.RequestChapterAsync(1);
        
        // 過場動畫淡出
        await loadingView.FadeOut();
        
        // 無縫進入章節閱讀
        var chapterView = await UIManager.Instance.OpenAsync<ChapterReadingView>();
        chapterView.DisplayChapter(firstChapter);
    }
    catch (Exception ex)
    {
        loadingView.ShowError($"第一章生成失敗：{ex.Message}");
    }
    finally
    {
        chapterService.OnGenerationProgress -= loadingView.UpdateProgress;
    }
}
```

**CultivationLoadingView 實作**
```csharp
// UI/Views/CultivationLoadingView.cs
public class CultivationLoadingView : UIView
{
    private Label messageLabel;
    private ProgressBar progressBar;
    private VisualElement backgroundAnimation;
    
    private string[] loadingMessages = new[] {
        "道途開啟...",
        "引氣入體...",
        "感知天地靈氣...",
        "凝聚靈氣...",
        "開啟修煉之路..."
    };
    
    private int currentMessageIndex = 0;
    
    public void SetMessage(string message)
    {
        messageLabel.text = message;
    }
    
    public void UpdateProgress(float progress)
    {
        progressBar.value = progress;
        
        // 根據進度切換訊息
        int messageIndex = Mathf.FloorToInt(progress * loadingMessages.Length);
        if (messageIndex != currentMessageIndex && messageIndex < loadingMessages.Length)
        {
            currentMessageIndex = messageIndex;
            SetMessage(loadingMessages[messageIndex]);
        }
    }
    
    public async UniTask FadeOut()
    {
        // 淡出動畫
        await DOTween.To(() => Root.style.opacity.value, x => Root.style.opacity = x, 0, 0.5f).AsyncWaitForCompletion();
    }
}
```

### 完整流程總結

```
階段一：刷命格（0 成本，無限次）
  CalculateDestiny RPC × N 次
  ↓
階段二：鎖定並生成故事（1 次 AI 成本）
  ConfirmDestiny RPC → 加入佇列
  GetDestinyStatus RPC → 輪詢
  DestinyWorker → 生成命格故事
  ↓
階段三：沉浸式載入第一章（1 次 AI 成本）
  RequestChapter(1) RPC → 加入佇列
  GetChapterStatus RPC → 輪詢
  ChapterWorker → 生成第一章
  ↓
無縫進入遊戲主循環
```

### 優勢分析

| 面向 | 優勢 |
|------|------|
| **成本控制** | 只有 2 次 AI 呼叫（命格故事 + 第一章），刷命格無成本 |
| **使用者體驗** | 刷命格快速有趣，兩次等待用沉浸式動畫包裝 |
| **防作弊** | 伺服器驗證數據一致性，防止竄改 |
| **留存率** | Gacha 玩法增加初期參與度 |
| **數據洞察** | 追蹤重刷次數，了解玩家偏好 |

### Firebase Analytics 事件

```csharp
// 刷命格階段
AnalyticsManager.Instance.LogEvent("destiny_calculated", new {
    spiritual_root = result.spiritual_root,
    quality = result.spiritual_root_quality
});

AnalyticsManager.Instance.LogEvent("destiny_rerolled", new {
    count = rerollCount,
    final_root = finalRoot
});

// 鎖定階段
AnalyticsManager.Instance.LogEvent("destiny_confirmed", new {
    spiritual_root = currentDestiny.spiritual_root,
    reroll_count = rerollCount,
    time_spent = timeSpent
});

// 故事生成完成
AnalyticsManager.Instance.LogEvent("destiny_story_generated", new {
    generation_time = generationTime,
    tokens_used = tokensUsed
});
```

### 數據驗證機制

```go
// 防止玩家竄改五行數據
func validateDestinyData(req ConfirmDestinyRequest) error {
    // 重新計算五行
    calculated := calculateFiveElements(req.BirthDate)
    
    // 檢查總和（應該是 100）
    sum := req.FiveElements.Metal + req.FiveElements.Wood + 
           req.FiveElements.Water + req.FiveElements.Fire + 
           req.FiveElements.Earth
    
    if sum != 100 {
        return errors.New("invalid five elements sum")
    }
    
    // 檢查靈根是否匹配
    expectedRoot := determineSpiritualRoot(req.FiveElements)
    if expectedRoot != req.SpiritualRoot {
        return errors.New("spiritual root mismatch")
    }
    
    // 檢查數值範圍（每個元素 0-100）
    if !isValidRange(req.FiveElements) {
        return errors.New("invalid element values")
    }
    
    return nil
}
```

