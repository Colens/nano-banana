import requests
import base64
import json

def test_img2img():
    # 读取图片并转为 base64
    with open("storage/local/7c149060-1c92-49ef-b32f-d073ad52b293.jpg", "rb") as f:
        img_base64 = base64.b64encode(f.read()).decode('utf-8')

    url = "http://localhost:8080/api/v1/tasks/generate"
    payload = {
        "prompt": "龟兔赛跑，把兔子改成马",
        "aspect_ratio": "4:3",
        "resolution_level": "2K",
        "reference_images": [img_base64]
    }

    try:
        response = requests.post(url, json=payload)
        print(f"Status Code: {response.status_code}")
        print(f"Response: {response.text}")
    except Exception as e:
        print(f"Error: {e}")

if __name__ == "__main__":
    test_img2img()
