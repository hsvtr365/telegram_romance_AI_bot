
import requests
import time
import json

url = "https://gitlab.swempire.co.kr/ollama/api/chat"
model = "gemma4-heretic:q4km"

payload = {
    "model": model,
    "messages": [
        {"role": "system", "content": "You analyze Korean chat context and return JSON only. No prose."},
        {"role": "user", "content": "Recent Conversation:\nassistant: 메뉴 골라보자.\nuser: 졸립다\n\nAnalyze this and return topic slots and conversation state."}
    ],
    "stream": False
}

print(f"Testing response time for model: {model} at {url}...")
start_time = time.time()
try:
    response = requests.post(url, json=payload, timeout=10)
    end_time = time.time()
    
    elapsed = end_time - start_time
    if response.status_code == 200:
        result = response.json()
        content = result.get("message", {}).get("content", "")
        print(f"Success! Time taken: {elapsed:.2f} seconds")
        print(f"Content length: {len(content)} characters")
        print(f"Response:\n{content}")
    else:
        print(f"Failed! Status code: {response.status_code}")
        print(response.text)
except Exception as e:
    print(f"Error during request: {e}")
