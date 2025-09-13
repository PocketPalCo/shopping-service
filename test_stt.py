#!/usr/bin/env python3

import requests
import json

def test_stt_service():
    """Test STT service with language auto-detection"""
    url = "http://localhost:8000/chunk/"

    # Test data - we'll use a simple POST without language parameters
    data = {
        'session_id': 'test_session_123',
        'chunk_id': 1,
        # Note: not sending language or target_language to test auto-detection
    }

    # Create a dummy audio file for testing (empty for now)
    files = {
        'file': ('test_audio.ogg', b'dummy audio data', 'audio/ogg')
    }

    try:
        print("🔄 Testing STT service with language auto-detection...")
        print(f"📡 Sending request to: {url}")
        print(f"📋 Request data: {data}")

        response = requests.post(url, data=data, files=files, timeout=30)

        print(f"📊 Response status: {response.status_code}")

        if response.status_code == 200:
            result = response.json()
            print("✅ STT service response:")
            print(json.dumps(result, indent=2))

            # Check if detected_language is present
            if 'detected_language' in result:
                print(f"🎯 Language detection working! Detected: {result['detected_language']}")
            else:
                print("⚠️ detected_language field missing in response")
        else:
            print(f"❌ Request failed with status {response.status_code}")
            print(f"Error: {response.text}")

    except requests.exceptions.RequestException as e:
        print(f"❌ Connection error: {e}")

if __name__ == "__main__":
    test_stt_service()