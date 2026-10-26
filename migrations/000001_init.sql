-- +goose Up
-- 服务商
CREATE TABLE x_service_provider
(
    id               TEXT PRIMARY KEY,               -- id 主键
    service_provider TEXT     NOT NULL,              -- 服务商名称
    setting_json     TEXT     NOT NULL DEFAULT '{}', -- JSON 配置
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_x_service_provider_name UNIQUE (service_provider)
);

-- 会话表
CREATE TABLE x_conversations
(
    id                TEXT PRIMARY KEY,               -- id 主键
    title             TEXT     NOT NULL DEFAULT '',   -- 会话标题
    message_count     INTEGER  NOT NULL DEFAULT 0,    -- 会话内消息数量
    last_message_time DATETIME          DEFAULT NULL, -- 最后消息时间
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_x_conversations_last_message_time ON x_conversations (last_message_time DESC);

-- 会话摘要
CREATE TABLE x_conversation_summaries
(
    id              TEXT PRIMARY KEY,             -- id 主键
    conversation_id TEXT     NOT NULL,            -- 会话 id
    content         TEXT     NOT NULL DEFAULT '', -- 摘要内容
    message_id      TEXT,                         -- 消息 id：触发摘要时的消息 id
    message_seq     INTEGER,                      -- 消息顺序：触发摘要时的消息顺序
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_x_summary_conversation UNIQUE (conversation_id)
);

-- 消息
CREATE TABLE x_messages
(
    id                TEXT PRIMARY KEY,                                                          -- id 主键
    conversation_id   TEXT     NOT NULL,                                                         -- 会话id
    seq               INTEGER  NOT NULL,                                                         -- 消息顺序
    role              TEXT     NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool')), -- 消息角色
    content           TEXT     NOT NULL,                                                         -- 消息内容
    service_provider  TEXT     NOT NULL DEFAULT '',                                              -- 服务商名称
    model             TEXT     NOT NULL DEFAULT '',                                              -- 模型名称
    thinking_content  TEXT     NOT NULL DEFAULT '',                                              -- 思考内容
    thinking_duration INTEGER  NOT NULL DEFAULT 0,                                               -- 思考时间 毫秒
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_x_messages_conversation_seq UNIQUE (conversation_id, seq)
);

-- +goose Down
DROP TABLE IF EXISTS x_conversation_summaries;
DROP TABLE IF EXISTS x_messages;
DROP TABLE IF EXISTS x_conversations;
DROP TABLE IF EXISTS x_service_provider;
