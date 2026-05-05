def can_process(payload, headers):
    # Check for GitHub-specific headers or payload structure
    return headers.get('X-GitHub-Event') is not None or 'repository' in payload and 'pusher' in payload

def process(payload):
    repo_name = payload.get('repository', {}).get('full_name', 'Unknown Repo')
    pusher = payload.get('pusher', {}).get('name', 'Unknown User')
    commits = payload.get('commits', [])
    summary = f"GitHub Push to {repo_name} by {pusher} ({len(commits)} commits)"
    
    return {
        "category": "GitHub Push",
        "summary": summary,
        "action_required": len(commits) > 0,
        "details": [c.get('message') for c in commits]
    }
