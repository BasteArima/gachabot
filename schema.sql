-- 1. Таблица Редкостей
CREATE TABLE rarities (
                          id SERIAL PRIMARY KEY,
                          name VARCHAR(50) NOT NULL UNIQUE,
                          drop_chance NUMERIC(5, 2) NOT NULL,
                          base_reward INTEGER NOT NULL DEFAULT 0,
                          pity_threshold INTEGER NOT NULL DEFAULT 0
);

-- 2. Таблица Карточек
CREATE TABLE cards (
                       id SERIAL PRIMARY KEY,
                       name VARCHAR(100) NOT NULL,
                       rarity_id INTEGER NOT NULL REFERENCES rarities(id) ON DELETE RESTRICT,
                       image_url TEXT,
                       power_level INTEGER NOT NULL DEFAULT 1
);

-- 3. Таблица Пользователей (Игроков) - ГЛОБАЛЬНАЯ
CREATE TABLE users (
                       id BIGSERIAL PRIMARY KEY, -- Единый внутренний ID бота
                       telegram_id BIGINT UNIQUE, -- ID в Telegram (может быть NULL, если юзер только из Discord)
                       discord_id BIGINT UNIQUE, -- ID в Discord (может быть NULL)

    -- Общая информация и прогресс
                       username VARCHAR(100),
                       first_name TEXT,
                       last_name TEXT,
                       balance INTEGER NOT NULL DEFAULT 0 CHECK (balance >= 0),
                       streak_days INTEGER NOT NULL DEFAULT 0,
                       last_roll_time TIMESTAMP WITH TIME ZONE,
                       last_streak_date DATE,
                       premium_rolls INTEGER NOT NULL DEFAULT 0,
                       language_code VARCHAR(10) DEFAULT '',
                       created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 4. Таблица Инвентаря (Связь Игроков и Карточек)
CREATE TABLE user_inventory (
                                user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- Ссылаемся на внутренний id
                                card_id INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
                                quantity INTEGER NOT NULL DEFAULT 1,
                                PRIMARY KEY (user_id, card_id)
);

-- 5. Таблица Осколков (Для крафта Мифических карт)
CREATE TABLE user_fragments (
                                user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- Ссылаемся на внутренний id
                                card_id INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
                                quantity INTEGER NOT NULL DEFAULT 1,
                                PRIMARY KEY (user_id, card_id)
);

-- 6. Таблица Гарантов (Система Pity)
CREATE TABLE user_pity (
                           user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- Ссылаемся на внутренний id
                           rarity_id INTEGER NOT NULL REFERENCES rarities(id) ON DELETE CASCADE,
                           counter INTEGER NOT NULL DEFAULT 0,
                           PRIMARY KEY (user_id, rarity_id)
);

-- 7. Таблица активности в чатах (Для локальных топов)
CREATE TABLE user_chats (
                            user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- Ссылаемся на внутренний id
                            chat_id BIGINT NOT NULL,
                            PRIMARY KEY (user_id, chat_id)
);