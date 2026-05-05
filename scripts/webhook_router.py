import os
import sys
import json
import importlib.util

PROCESSORS_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'processors')

def load_processor(file_path):
    spec = importlib.util.spec_from_file_location("processor", file_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module

def main():
    if len(sys.argv) < 2:
        print("Usage: python webhook_router.py <payload_json> [headers_json]")
        sys.exit(1)

    payload_str = sys.argv[1]
    headers_str = sys.argv[2] if len(sys.argv) > 2 else "{}"

    try:
        payload = json.loads(payload_str)
        headers = json.loads(headers_str)
    except json.JSONDecodeError:
        print("Error: Invalid JSON input")
        sys.exit(1)

    if not os.path.exists(PROCESSORS_DIR):
        os.makedirs(PROCESSORS_DIR)

    matched_processor = None
    for filename in os.listdir(PROCESSORS_DIR):
        if filename.endswith('.py'):
            file_path = os.path.join(PROCESSORS_DIR, filename)
            try:
                processor = load_processor(file_path)
                if hasattr(processor, 'can_process') and processor.can_process(payload, headers):
                    matched_processor = processor
                    break
            except Exception as e:
                print(f"Error loading processor {filename}: {e}", file=sys.stderr)

    if matched_processor:
        try:
            result = matched_processor.process(payload)
            print(json.dumps(result, indent=2))
            sys.exit(0)
        except Exception as e:
            print(f"Error executing processor: {e}", file=sys.stderr)
            sys.exit(1)
    else:
        print("No deterministic processor found.")
        sys.exit(2) # Status 2 signals "Codex intervention needed"

if __name__ == "__main__":
    main()
