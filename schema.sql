-- 1. Таблица Редкостей
CREATE TABLE rarities (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    drop_chance NUMERIC(5, 2) NOT NULL,
    base_reward INTEGER NOT NULL DEFAULT 0,
    pity_threshold INTEGER NOT NULL DEFAULT 0,
    craft_cost INTEGER NOT NULL DEFAULT 0,
    -- A normal roll of this rarity yields a fragment instead of the card itself.
    requires_fragments BOOLEAN NOT NULL DEFAULT FALSE,
    -- How many fragments are needed to assemble a card of this rarity.
    fragments_required INTEGER NOT NULL DEFAULT 0
);

-- 2. Таблица коллекций (объявляется до cards и users, т.к. на неё ссылаются их FK)
CREATE TABLE IF NOT EXISTS card_sets (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    buff_type VARCHAR(50) NOT NULL,
    buff_value INT NOT NULL DEFAULT 0,
    reward_points INT NOT NULL DEFAULT 0
    );

-- 3. Таблица Карточек
CREATE TABLE cards (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    rarity_id INTEGER NOT NULL REFERENCES rarities(id) ON DELETE RESTRICT,
    image_url TEXT,
    power_level INTEGER NOT NULL DEFAULT 1,
    set_id INTEGER REFERENCES card_sets(id) ON DELETE SET NULL
);

-- 4. Таблица Пользователей (Игроков) - ГЛОБАЛЬНАЯ
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY, -- Единый внутренний ID бота
    telegram_id BIGINT UNIQUE, -- ID в Telegram (может быть NULL, если юзер только из Discord)
    discord_id BIGINT UNIQUE, -- ID в Discord (может быть NULL)

    username VARCHAR(100),
    first_name TEXT,
    last_name TEXT,
    balance INTEGER NOT NULL DEFAULT 0 CHECK (balance >= 0),
    streak_days INTEGER NOT NULL DEFAULT 0,
    last_roll_time TIMESTAMP WITH TIME ZONE,
    last_streak_date DATE,
    premium_rolls INTEGER NOT NULL DEFAULT 0,
    language_code VARCHAR(10) DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    active_set_id INT REFERENCES card_sets(id) ON DELETE SET NULL,
    is_adult BOOLEAN DEFAULT NULL
);

-- 5. Таблица Инвентаря (Связь Игроков и Карточек)
CREATE TABLE user_inventory (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    card_id INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    quantity INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, card_id)
);

-- 6. Таблица Осколков (для крафта Мифических карт)
CREATE TABLE user_fragments (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    card_id INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    quantity INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, card_id)
);

-- 7. Таблица Гарантов
CREATE TABLE user_pity (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rarity_id INTEGER NOT NULL REFERENCES rarities(id) ON DELETE CASCADE,
    counter INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, rarity_id)
);

-- 8. Таблица активности в чатах
CREATE TABLE user_chats (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_id BIGINT NOT NULL,
    PRIMARY KEY (user_id, chat_id)
);

-- 9. Таблица промокодов
CREATE TABLE IF NOT EXISTS promocodes (
    code VARCHAR(50) PRIMARY KEY,
    reward_json JSONB NOT NULL,
    max_uses INT DEFAULT NULL,
    current_uses INT DEFAULT 0,
    expires_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
    );

-- 10. Таблица использованных промокодов
CREATE TABLE IF NOT EXISTS promocode_usages (
    user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    promocode VARCHAR(50) REFERENCES promocodes(code) ON DELETE CASCADE,
    used_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (user_id, promocode)
    );

-- 11. Таблица отслеживания разблокированных сетов и прогресса
CREATE TABLE IF NOT EXISTS user_unlocked_sets (
    user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    set_id INT REFERENCES card_sets(id) ON DELETE CASCADE,
    is_completed BOOLEAN DEFAULT FALSE,
    UNIQUE(user_id, set_id)
    );
