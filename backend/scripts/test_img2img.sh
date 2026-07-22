#!/bin/bash

# 读取图片并转为 base64
IMG_BASE64=$(base64 -i storage/local/7c149060-1c92-49ef-b32f-d073ad52b293.jpg)

# 构造 JSON
cat <<EOF > scripts/payload.json
{
    "provider": "gemini",
    "params": {
        "prompt": "龟兔赛跑，把兔子改成马",
        "aspect_ratio": "4:3",
        "resolution_level": "2K",
        "reference_images": ["$IMG_BASE64"]
    }
}
EOF

# 发送请求
curl -X POST http://localhost:8080/api/v1/tasks/generate \
     -H "Content-Type: application/json" \
     -d @scripts/payload.json
