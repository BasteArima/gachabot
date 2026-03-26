-- 1. Создаем таблицу Редкостей
CREATE TABLE rarities (
                          id SERIAL PRIMARY KEY,
                          name VARCHAR(50) NOT NULL UNIQUE,
                          drop_chance DECIMAL(5, 2) NOT NULL, -- например, 2.50 (%)
                          base_reward INT NOT NULL DEFAULT 0
);

-- 2. Создаем таблицу Карточек
CREATE TABLE cards (
                       id SERIAL PRIMARY KEY,
                       name VARCHAR(100) NOT NULL,
                       rarity_id INT NOT NULL REFERENCES rarities(id) ON DELETE RESTRICT,
                       image_url TEXT,
                       power_level INT NOT NULL DEFAULT 1
);

-- 3. Создаем таблицу Игроков
CREATE TABLE users (
                       tg_id BIGINT PRIMARY KEY,
                       username VARCHAR(100),
                       balance INT NOT NULL DEFAULT 0 CHECK (balance >= 0),
                       streak_days INT NOT NULL DEFAULT 0,
                       last_roll_time TIMESTAMP WITH TIME ZONE,
                       last_streak_date DATE,
                       created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 4. Создаем таблицу Инвентаря (связь Игроков и Карточек)
CREATE TABLE user_inventory (
                                user_id BIGINT REFERENCES users(tg_id) ON DELETE CASCADE,
                                card_id INT REFERENCES cards(id) ON DELETE CASCADE,
                                quantity INT NOT NULL DEFAULT 1,
                                PRIMARY KEY (user_id, card_id) -- Составной ключ
);