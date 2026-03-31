import os
import sys
from PIL import Image

TARGET_SIZE = (1000, 1316)
# Получаем путь к папке, в которой лежит сам gen.py
BASE_DIR = os.path.dirname(os.path.abspath(__file__))

# Теперь строим пути относительно BASE_DIR
RAW_DIR = os.path.join(BASE_DIR, 'raw_art')
FRAME_DIR = os.path.join(BASE_DIR, 'frames')
OUT_DIR = os.path.join(BASE_DIR, 'out')

# ВНИМАНИЕ: Проверь, чтобы названия в этом списке В ТОЧНОСТИ (включая регистр) 
# совпадали с названиями твоих папок в raw_art
RARITIES = [
    #"Common", 
    #"Uncommon", 
    "Rare", 
    #"Epic", 
    #"Legendary",
    #"Mythical"
    ]

def process_image(img_path, frame_path, save_path):
    try:
        with Image.open(img_path).convert("RGBA") as base, \
             Image.open(frame_path).convert("RGBA") as frame:
            
            target_ratio = TARGET_SIZE[0] / TARGET_SIZE[1]
            base_w, base_h = base.size
            base_ratio = base_w / base_h

            if base_ratio > target_ratio:
                new_width = int(target_ratio * base_h)
                offset = (base_w - new_width) // 2
                base = base.crop((offset, 0, offset + new_width, base_h))
            else:
                new_height = int(base_w / target_ratio)
                offset = (base_h - new_height) // 2
                base = base.crop((0, offset, base_w, offset + new_height))

            base = base.resize(TARGET_SIZE, Image.Resampling.LANCZOS)
            combined = Image.alpha_composite(base, frame)
            combined.save(save_path, "WEBP", quality=90)
            return True
    except Exception as e:
        print(f"❌ Ошибка обработки {img_path}: {e}")
        return False

def main():
    print("🔍 Запуск сканирования...")
    
    # Проверка базовых папок
    for d in [RAW_DIR, FRAME_DIR]:
        if not os.path.exists(d):
            print(f"❗ КРИТИЧЕСКАЯ ОШИБКА: Папка '{d}' не найдена рядом со скриптом!")
            return

    # Посмотрим, какие папки реально есть в raw_art
    actual_folders = os.listdir(RAW_DIR)
    print(f"📁 В папке {RAW_DIR} найдено подпапок: {actual_folders}")

    processed_count = 0

    for rarity in RARITIES:
        rarity_raw_path = os.path.join(RAW_DIR, rarity)
        rarity_out_path = os.path.join(OUT_DIR, rarity)
        frame_file = os.path.join(FRAME_DIR, f"{rarity}.png")

        print(f"\n--- Проверка редкости: {rarity} ---")

        if not os.path.exists(rarity_raw_path):
            print(f"⚠️ Папка '{rarity_raw_path}' НЕ НАЙДЕНА. Пропускаю.")
            continue

        if not os.path.exists(frame_file):
            print(f"⚠️ Файл рамки '{frame_file}' НЕ НАЙДЕН. Пропускаю.")
            continue

        if not os.path.exists(rarity_out_path):
            os.makedirs(rarity_out_path)

        files = [f for f in os.listdir(rarity_raw_path) if f.lower().endswith(('.png', '.jpg', '.jpeg', '.webp'))]
        print(f"🔎 Найдено изображений: {len(files)}")

        for filename in files:
            name_only = os.path.splitext(filename)[0]
            img_src = os.path.join(rarity_raw_path, filename)
            img_dst = os.path.join(rarity_out_path, f"{name_only}.webp")

            if process_image(img_src, frame_file, img_dst):
                print(f"✅ Готово: {name_only}.webp")
                processed_count += 1

    print(f"\n🚀 Итог: обработано {processed_count} изображений.")

if __name__ == "__main__":
    main()