import os
import time
import requests
import re

# Папка, где запущен скрипт
BASE_DIR = os.path.dirname(os.path.abspath(__file__))

def sanitize_filename(name):
    """Удаляет из тега символы, которые нельзя использовать в названиях файлов Windows"""
    return re.sub(r'[\\/*?:"<>|]', "", name)

def download_image(tag, folder_path):
    """Ищет самый популярный арт по тегу и скачивает его"""
    # Формируем запрос: сам тег + сортировка по очкам + исключаем видео
    query_tag = f"{tag} sort:score:desc -video"
    
    # URL для API Rule34 (запрашиваем JSON, лимит 1 пост)
    url = f"https://api.rule34.xxx/index.php?page=dapi&s=post&q=index&json=1&limit=1&tags={query_tag}"
    
    # Фейковый User-Agent, чтобы API не блокировал скрипт
    headers = {'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36'}
    
    try:
        response = requests.get(url, headers=headers, timeout=10)
        
        # Проверяем, что сервер ответил нормально
        if response.status_code != 200:
            print(f"  [❌ Ошибка API] Код {response.status_code}")
            return
            
        data = response.json()
        
        # Если по тегу ничего не найдено
        if not data:
            print(f"  [⚠️ Пусто] Арты не найдены")
            return
            
        post = data[0]
        file_url = post.get('file_url')
        
        if not file_url:
            print(f"  [⚠️ Ошибка] Нет ссылки на файл")
            return
            
        # Достаем расширение файла (jpg, png, jpeg)
        ext = file_url.split('.')[-1]
        safe_tag = sanitize_filename(tag)
        
        # Итоговый путь к картинке
        filepath = os.path.join(folder_path, f"{safe_tag}.{ext}")
        
        # Если мы уже качали эту картинку, пропускаем (чтобы можно было перезапускать скрипт)
        if os.path.exists(filepath):
            print(f"  [⏩ Пропуск] Уже скачан: {safe_tag}.{ext}")
            return
            
        # Скачиваем саму картинку
        img_data = requests.get(file_url, headers=headers, timeout=15).content
        with open(filepath, 'wb') as f:
            f.write(img_data)
            
        print(f"  [✅ Успех] Скачан: {safe_tag}.{ext} (Score: {post.get('score', 0)})")
        
    except Exception as e:
        print(f"  [❌ Ошибка соединения] {e}")

def main():
    # Ищем все txt файлы, в названии которых есть слово 'rarity'
    txt_files = [f for f in os.listdir(BASE_DIR) if f.endswith('.txt') and 'rarity' in f]
    
    if not txt_files:
        print("❌ Файлы с тегами (rarity_*.txt) не найдены в папке со скриптом!")
        return
        
    for txt_file in txt_files:
        # Имя папки будет таким же, как имя txt файла, только без .txt
        folder_name = txt_file.replace('.txt', '')
        folder_path = os.path.join(BASE_DIR, folder_name)
        
        # Создаем папку, если ее нет
        if not os.path.exists(folder_path):
            os.makedirs(folder_path)
            
        print(f"\n📂=== Чтение файла: {txt_file} ===📂")
        
        # Читаем теги
        with open(os.path.join(BASE_DIR, txt_file), 'r', encoding='utf-8') as f:
            tags = [line.strip() for line in f if line.strip()]
            
        for i, tag in enumerate(tags):
            print(f"[{i+1}/{len(tags)}] Тег: {tag}")
            download_image(tag, folder_path)
            
            # ВАЖНО: Задержка 1.5 секунды между запросами, чтобы Rule34 не забанил твой IP
            time.sleep(1.5)

    print("\n🎉 Все файлы обработаны!")

if __name__ == "__main__":
    main()