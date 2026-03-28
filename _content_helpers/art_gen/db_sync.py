import os

# --- НАСТРОЙКИ ---
BASE_URL = "https://api.baste.ru/cards" 

# Маппинг: Название папки -> (ID редкости в БД, Базовая сила)
RARITY_CONFIG = {
    #"Common":    (1, 10),
    "Uncommon":  (2, 50),
    #"Rare":     (3, 150),
    #"Epic":  (4, 400),
    #"Legendary": (5, 800),
    #"Mythical": (6, 2000)
}

# --- АВТО-ПУТИ ---
# Определяем папку, где лежит сам скрипт
BASE_DIR = os.path.dirname(os.path.abspath(__file__))

# Путь к папке out и итоговому SQL файлу теперь всегда привязан к папке скрипта
OUT_DIR = os.path.join(BASE_DIR, 'out')
SQL_OUTPUT = os.path.join(BASE_DIR, 'import_cards.sql')

def generate_sql():
    sql_statements = []
    
    if not os.path.exists(OUT_DIR):
        print(f"❌ ОШИБКА: Папка '{OUT_DIR}' не найдена!")
        print("Сначала запусти gen.py, чтобы создать папку 'out'.")
        return

    processed_total = 0

    for rarity_name, (rarity_id, power) in RARITY_CONFIG.items():
        folder_path = os.path.join(OUT_DIR, rarity_name)
        
        if not os.path.exists(folder_path):
            continue

        # Берем только .webp файлы
        files = [f for f in os.listdir(folder_path) if f.lower().endswith('.webp')]
        if not files:
            continue

        print(f"📦 Обработка {rarity_name}: {len(files)} карт")

        for filename in files:
            # Имя карты: убираем расширение, заменяем подчеркивания на пробелы, делаем заглавным
            card_name = os.path.splitext(filename)[0].replace('_', ' ').capitalize()
            card_name_escaped = card_name.replace("'", "''")
            
            # Формируем URL для базы данных
            image_url = f"{BASE_URL}/{rarity_name}/{filename}"
            
            # SQL Инсерт
            stmt = f"INSERT INTO cards (name, rarity_id, image_url, power_level) VALUES ('{card_name_escaped}', {rarity_id}, '{image_url}', {power});"
            sql_statements.append(stmt)
            processed_total += 1

    # Записываем в файл
    try:
        with open(SQL_OUTPUT, 'w', encoding='utf-8') as f:
            f.write("\n".join(sql_statements))
        
        print(f"\n🚀 УСПЕХ! Обработано {processed_total} карт.")
        print(f"📍 Файл создан по адресу: {SQL_OUTPUT}")
        print(f"💡 Теперь просто открой его и выполни в консоли БД.")
    except Exception as e:
        print(f"❌ Ошибка записи файла: {e}")

if __name__ == "__main__":
    generate_sql()