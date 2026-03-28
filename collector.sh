#!/bin/bash

# По умолчанию: текущая директория и output.txt
DIR="${1:-$HOME/GolandProjects/Videocall-Chat-GO-Project}"
OUTPUT="${2:-output.txt}"

EXCLUDES=(
    "assets"
    "go.mod"
    "go.sum"
    ".git"
    "README.md"
    ".gitignore"
    ".gitmodules"
    "LICENSE"
    ".idea"
    "collector.sh"
    ".venv"
    "practice"
    "noisecanceletionmodel/data"
    "noisecanceletionmodel/venv"
    "noisecanceletionmodel/speech-noise-dataset.zip"
    "views"
)

TEMP_OUTPUT=$(mktemp)

rm -f "$OUTPUT"

normalize_path() {
    echo "$1" | sed 's|/$||; s|^./||'
}

# Формируем параметры для find
FIND_EXCLUDES=()
for EX in "${EXCLUDES[@]}"; do
    FIND_EXCLUDES+=(-path "$DIR/$EX" -prune -o)
done

eval find "\"$DIR\"" "${FIND_EXCLUDES[@]}" -type f -print0 | while IFS= read -r -d '' FILE; do
    REL_PATH=$(realpath --relative-to="$DIR" "$FILE" 2>/dev/null || echo "$FILE" | sed "s|^$DIR/||")

    REL_PATH=$(normalize_path "$REL_PATH")

    echo "# $REL_PATH" >> "$TEMP_OUTPUT"
    if cat "$FILE" | awk '{print} END{print ""}' >> "$TEMP_OUTPUT"; then
        :
    else
        echo "Error: Failed to read $FILE" >&2
    fi
done

mv "$TEMP_OUTPUT" "$OUTPUT"

echo "All files collected into $OUTPUT"