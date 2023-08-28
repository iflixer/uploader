# uploader

веб-интерфейс
аплоад видеофайла
загрузка файла с торрента по указанной ссылке
список текущих и завершенных задач


конвертация видеофайла в нужные форматы
загрузка файла в S3


DB
tasks
url
created
status


get probe:
http://127.0.0.1:9000/probe?videofile=test.mp4
add to convertion queue:
http://127.0.0.1:9000/convert?videofile=test2.mp4
check status:
http://127.0.0.1:9000/status?videofile=test2.mp4
list:
http://127.0.0.1:9000/list?videofile=test2.mp4

original script:
```bash
#-map 0:0 -map 0:1 \
# ./script.sh video,mkv 1100

bitrate="$(awk "BEGIN {print int($2 * 1024 * 1024 * 8 / $(ffprobe \
    -v error \
    -show_entries format=duration \
    -of default=noprint_wrappers=1:nokey=1 \
    "$1" \
) / 1000)}")k"
ffmpeg \
    -y \
    -i "$1" \
    -c:v libx264 \
    -preset medium \
    -b:v $bitrate \
    -pass 1 \
    -map 0:0 -map 0:1 \
        -c:a:0 aac -b:a:0 192k -ac 2 \
        -c:a:1 copy \
        -s 640x360 \
    -f mp4 \
    /dev/null \
&& \
ffmpeg \
    -i "$1" \
    -c:v libx264 \
    -preset medium \
    -b:v $bitrate \
    -pass 2 \
    -map 0:0 -map 0:1 \
        -c:a:0 aac -b:a:0 192k -ac 2 \
        -c:a:1 copy \
        -s 640x360 \
    -movflags faststart \
    "${1%.*}-$2mB.mp4"


```