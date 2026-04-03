import psycopg2

# Настройки подключения к твоей локальной БД в Docker
DB_HOST = "localhost"
DB_PORT = "5432"
DB_NAME = "gachabot"
DB_USER = "root"       # Замени, если у тебя другой юзер (например, postgres)
DB_PASS = "secretpassword"     # Твой пароль от БД

def connect():
    return psycopg2.connect(
        host=DB_HOST, port=DB_PORT, dbname=DB_NAME, user=DB_USER, password=DB_PASS
    )

def create_set(conn):
    print("\n--- СОЗДАНИЕ НОВОГО СЕТА ---")
    name = input("Название сета (например, 'Пляжный эпизод'): ")
    buff_type = input("Тип баффа (сейчас доступен только 'power_percent'): ") or "power_percent"
    buff_value = input("Значение баффа (например, 5 для 5%): ")
    reward_points = input("Награда за сбор в очках (например, 5000): ")

    with conn.cursor() as cur:
        cur.execute(
            "INSERT INTO card_sets (name, buff_type, buff_value, reward_points) VALUES (%s, %s, %s, %s) RETURNING id;",
            (name, buff_type, int(buff_value), int(reward_points))
        )
        set_id = cur.fetchone()[0]
        conn.commit()
        print(f"✅ Сет '{name}' успешно создан! Его ID: {set_id}")
        return set_id

def assign_cards(conn, set_id):
    print(f"\n--- ПРИВЯЗКА КАРТ К СЕТУ ID:{set_id} ---")
    cards_input = input("Введи ID карточек через запятую (например: 12, 45, 108): ")
    
    if not cards_input.strip():
        print("Отмена.")
        return

    card_ids = [int(x.strip()) for x in cards_input.split(",")]

    with conn.cursor() as cur:
        cur.execute(
            "UPDATE cards SET set_id = %s WHERE id = ANY(%s);",
            (set_id, card_ids)
        )
        updated = cur.rowcount
        conn.commit()
        print(f"✅ Успешно привязано карт: {updated} шт.")

def main():
    try:
        conn = connect()
    except Exception as e:
        print("❌ Ошибка подключения к БД:", e)
        return

    while True:
        print("\n=== УПРАВЛЕНИЕ КОЛЛЕКЦИЯМИ ===")
        print("1. Создать новый сет и сразу добавить в него карты")
        print("2. Добавить карты в уже существующий сет")
        print("3. Выход")
        choice = input("Выбери действие: ")

        if choice == '1':
            new_id = create_set(conn)
            assign_cards(conn, new_id)
        elif choice == '2':
            # Сначала покажем все сеты, чтобы вспомнить ID
            with conn.cursor() as cur:
                cur.execute("SELECT id, name FROM card_sets ORDER BY id;")
                sets = cur.fetchall()
                print("\nСуществующие сеты:")
                for s in sets:
                    print(f"ID: {s[0]} | Название: {s[1]}")
            
            set_id = input("\nВведи ID сета, в который хочешь добавить карты: ")
            if set_id.isdigit():
                assign_cards(conn, int(set_id))
        elif choice == '3':
            break

    conn.close()
    print("До встречи!")

if __name__ == "__main__":
    main()