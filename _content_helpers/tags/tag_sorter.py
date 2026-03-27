import os
import msvcrt
import sys

BASE_DIR = os.path.dirname(os.path.abspath(__file__))

LIMITS = {"1": 300, "2": 200, "3": 150, "4": 100, "5": 50, "6": 20}
FILES = {
    "1": "rarity_1_common.txt", "2": "rarity_2_uncommon.txt",
    "3": "rarity_3_rare.txt", "4": "rarity_4_epic.txt",
    "5": "rarity_5_legendary.txt", "6": "rarity_6_mythic.txt",
    "0": "trash.txt"
}

def load_all_tags():
    """Загружает исходный список всех тегов"""
    path = os.path.join(BASE_DIR, "r34_tags_top.txt")
    if not os.path.exists(path): return []
    with open(path, "r", encoding="utf-8") as f:
        return [line.strip() for line in f if line.strip()]

def init_and_load_progress():
    """Создает файлы, если их нет, и загружает уже отсортированные теги"""
    results = {}
    for k, filename in FILES.items():
        filepath = os.path.join(BASE_DIR, filename)
        # Если файла нет - создаем пустой
        if not os.path.exists(filepath):
            with open(filepath, "w", encoding="utf-8") as f:
                pass 
        else:
            # Если есть - читаем, что там уже лежит
            with open(filepath, "r", encoding="utf-8") as f:
                for line in f:
                    tag = line.strip()
                    if tag:
                        results[tag] = k
    return results

def save_realtime(results):
    """Мгновенно распределяет текущие результаты по txt файлам"""
    grouped = {k: [] for k in FILES.keys()}
    for t, r in results.items():
        grouped[r].append(t)
    
    for k, filename in FILES.items():
        filepath = os.path.join(BASE_DIR, filename)
        with open(filepath, "w", encoding="utf-8") as f:
            f.write("\n".join(grouped[k]))

def main():
    all_tags = load_all_tags()
    if not all_tags:
        print("❌ Файл r34_tags_top.txt не найден в папке со скриптом!")
        return

    # Загружаем прогресс из файлов
    results = init_and_load_progress()
    history = []  # стек для Undo (в рамках одной сессии)
    idx = 0

    # Проматываем индекс до первого неразобранного тега
    while idx < len(all_tags) and all_tags[idx] in results:
        idx += 1

    if idx >= len(all_tags):
        print("🎉 Все теги уже отсортированы!")
        return

    while idx < len(all_tags):
        tag = all_tags[idx]
        
        # Считаем текущую статистику
        stats = {k: 0 for k in FILES.keys()}
        for res in results.values():
            stats[res] += 1
        
        os.system('cls' if os.name == 'nt' else 'clear')
        print(f"--- Сортировщик Тегов [{idx+1}/{len(all_tags)}] ---")
        
        # Вывод категорий и лимитов
        for k, name in FILES.items():
            current_count = stats[k]
            limit = LIMITS.get(k, float('inf'))
            limit_display = LIMITS.get(k, '∞')
            
            # Если лимит превышен, показываем предупреждение (!)
            warning = " [ПРЕВЫШЕН ЛИМИТ!]" if current_count > limit else ""
            print(f"[{k}] {name.split('_')[-1]:<12} : {current_count}/{limit_display}{warning}")
        
        # Показываем контекст (предыдущий и следующий теги)
        prev_tag = all_tags[idx-1] if idx > 0 else ""
        next_tag = all_tags[idx+1] if idx < len(all_tags)-1 else ""
        
        print(f"\nПредыдущий: {prev_tag}")
        print(f"ТЕКУЩИЙ ТЕГ: >>> {tag} <<<")
        print(f"Следующий:  {next_tag}")
        
        prev_choice = results.get(tag)
        if prev_choice:
            print(f"\n[Уже назначено: {FILES[prev_choice]}]")
            print("Нажми [Enter] для подтверждения или новую цифру для перезаписи.")
        
        print("\n[1-6] Редкость | [0/T] Мусор | [Backspace] Назад | [ESC] Выход")

        # Читаем нажатие (только для Windows - msvcrt)
        char = msvcrt.getch()
        
        # ESC
        if char == b'\x1b': 
            print("\n💾 Выход... Все данные уже сохранены.")
            break
        
        # Backspace (Undo)
        if char == b'\x08':
            if history:
                idx = history.pop() # Возвращаемся на индекс назад
                tag_to_remove = all_tags[idx]
                # Удаляем тег из словаря и сразу обновляем файлы
                if tag_to_remove in results:
                    del results[tag_to_remove]
                    save_realtime(results)
            elif idx > 0:
                # Если история пуста (например, сразу после запуска), 
                # но мы можем откатиться на 1 шаг вручную
                idx -= 1
            continue

        # Enter (Пропуск / Подтверждение)
        if char == b'\r':
            if prev_choice:
                history.append(idx)
                idx += 1
            continue

        # Обработка цифр и букв
        try:
            key = char.decode('utf-8').lower()
            if key == 't': key = '0'
            if key in FILES:
                results[tag] = key
                save_realtime(results) # МГНОВЕННОЕ СОХРАНЕНИЕ
                history.append(idx)
                idx += 1
        except:
            continue

    print("\n✅ Работа завершена!")

if __name__ == "__main__":
    main()