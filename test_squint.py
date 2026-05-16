import requests
import json

def test_squint_ocr(image_url):
    # The local address where your Podman/Docker container is exposing Squint
    SQUINT_API_URL = "http://localhost:8080/api/v1/ocr"
    
    # Define the query parameters matching the Go handler
    payload = {"image_url": image_url}
    
    print(f"Sending image URL to Squint: {image_url}\n")
    
    try:
        # Fire off the GET request to the Go microservice
        response = requests.get(SQUINT_API_URL, params=payload, timeout=15)
        
        # Check if the HTTP request was successful (Status code 200)
        if response.status_code == 200:
            result = response.json()
            print("--- OCR RESULT RECEIVED ---")
            print(json.dumps(result, indent=4))
        else:
            print(f"Error: Server responded with status code {response.status_code}")
            try:
                print(json.dumps(response.json(), indent=4))
            except json.JSONDecodeError:
                print(response.text)
                
    except requests.exceptions.Timeout:
        print("Error: The request timed out. Is the Go service busy or frozen?")
    except requests.exceptions.ConnectionError:
        print("Error: Could not connect to Squint. Is the container running on port 8080?")
    except Exception as e:
        print(f"An unexpected error occurred: {e}")

if __name__ == "__main__":
    # Feel free to replace this with any direct image link that contains text
    sample_image = "https://raw.githubusercontent.com/otiai10/gosseract/main/test/data/001-helloworld.png"
    
    test_squint_ocr(sample_image)