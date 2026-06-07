import os
import re

def replace_in_file(filepath, replacements):
    with open(filepath, 'r') as f:
        content = f.read()
    
    for old, new in replacements:
        content = re.sub(old, new, content)
        
    with open(filepath, 'w') as f:
        f.write(content)

base_dir = '/Users/wangpei/src/singe/SuperTeam/apps/web/src'

replace_in_file(os.path.join(base_dir, 'features/teams/components/team-capabilities-tab.tsx'), [
    (r'human_owner_user_id: draft\?\.human_owner_user_id \?\? currentRevision\?\.human_owner_user_id,',
     r'human_owner_user_ids: draft?.human_owner_user_ids ?? currentRevision?.human_owner_user_ids ?? [],')
])

replace_in_file(os.path.join(base_dir, 'features/teams/components/team-governance-tab.tsx'), [
    (r'human_owner_user_id: sourceRevision\?\.human_owner_user_id,',
     r'human_owner_user_ids: sourceRevision?.human_owner_user_ids,')
])

replace_in_file(os.path.join(base_dir, 'features/teams/components/team-detail-layout.tsx'), [
    (r'if \(team\.human_owner\) \{\n\s+return team\.human_owner\.display_name \|\| team\.human_owner\.username \|\| team\.human_owner\.email \|\| team\.human_owner\.user_id;\n\s+\}',
     r'if (team.human_owners && team.human_owners.length > 0) {\n    const owner = team.human_owners[0];\n    return owner.display_name || owner.username || owner.email || owner.user_id;\n  }'),
    (r'return team\.human_owner_user_id \?\? "未设置";',
     r'return team.human_owner_user_ids?.join(", ") || "未设置";')
])

replace_in_file(os.path.join(base_dir, 'features/teams/components/team-card-grid.tsx'), [
    (r'if \(team\.human_owner\) \{\n\s+return \[\n\s+\{\n\s+avatar: team\.human_owner\.avatar,\n\s+display_name: team\.human_owner\.display_name,\n\s+email: team\.human_owner\.email,\n\s+id: team\.human_owner\.user_id,\n\s+status: team\.human_owner\.status,\n\s+username: team\.human_owner\.username,\n\s+\},\n\s+\];\n\s+\}',
     r'if (team.human_owners && team.human_owners.length > 0) {\n    return team.human_owners.map((owner) => ({\n      avatar: owner.avatar,\n      display_name: owner.display_name,\n      email: owner.email,\n      id: owner.user_id,\n      status: owner.status,\n      username: owner.username,\n    }));\n  }'),
    (r'if \(team\.human_owner_user_id\) \{\n\s+return \[\n\s+\{\n\s+id: team\.human_owner_user_id,\n\s+status: "active",\n\s+username: "Unknown",\n\s+\},\n\s+\];\n\s+\}',
     r'if (team.human_owner_user_ids && team.human_owner_user_ids.length > 0) {\n    return team.human_owner_user_ids.map(id => ({\n      id,\n      status: "active",\n      username: "Unknown",\n    }));\n  }')
])

replace_in_file(os.path.join(base_dir, 'features/teams/index.tsx'), [
    (r'human_owner_user_id: draft\.owner\?\.id \?\? "",',
     r'human_owner_user_ids: draft.owner?.id ? [draft.owner.id] : [],')
])

# For test files, simple regex replacements of human_owner_user_id -> human_owner_user_ids and human_owner -> human_owners
def fix_test_file(filepath):
    with open(filepath, 'r') as f:
        content = f.read()
    content = content.replace('human_owner_user_id:', 'human_owner_user_ids:').replace('human_owner:', 'human_owners:')
    # Fix the string vs array
    content = re.sub(r'human_owner_user_ids:\s*"([^"]+)",', r'human_owner_user_ids: ["\1"],', content)
    content = re.sub(r'human_owner_user_ids:\s*`([^`]+)`,', r'human_owner_user_ids: [`\1`],', content)
    # Fix the object vs array
    content = re.sub(r'human_owners:\s*{\s*([^}]+?)\s*},', r'human_owners: [{\1}],', content, flags=re.DOTALL)
    
    with open(filepath, 'w') as f:
        f.write(content)

fix_test_file(os.path.join(base_dir, 'features/teams/index.test.tsx'))
fix_test_file(os.path.join(base_dir, 'lib/api/teams.test.ts'))

