#!/bin/bash
cat devices/agent.sh | ssh -o StrictHostKeyChecking=no root@10.0.0.3 "cat > /tmp/agent.sh"
ssh -o StrictHostKeyChecking=no root@10.0.0.3 "sed -i 's/TU_API_KEY_AQUI/a29c9e4cb27c6a350b7d0d9dbf70ac03/g' /tmp/agent.sh"
ssh -o StrictHostKeyChecking=no root@10.0.0.3 "sed -i 's/10.0.0.6/10.0.0.6/g' /tmp/agent.sh"
ssh -o StrictHostKeyChecking=no root@10.0.0.3 "chmod +x /tmp/agent.sh && nohup /tmp/agent.sh > /dev/null 2>&1 &"
