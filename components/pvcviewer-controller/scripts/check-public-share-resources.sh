#!/bin/bash

# 檢查現存 PVCViewer 的公開分享資源
# 此腳本會驗證每個 PVCViewer 是否有正確的公開分享資源配置

set -e

# 顏色定義
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 計數器
total_pvcviewers=0
pvcviewers_with_networking=0
missing_resources=0
correct_resources=0

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}PVCViewer 公開分享資源檢查工具${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# 獲取所有 PVCViewer
echo -e "${BLUE}正在掃描所有 namespace 的 PVCViewer...${NC}"
pvcviewers=$(kubectl get pvcviewer --all-namespaces -o json)

# 解析 PVCViewer 列表
namespaces=$(echo "$pvcviewers" | jq -r '.items[].metadata.namespace' | sort -u)

if [ -z "$namespaces" ]; then
    echo -e "${YELLOW}未找到任何 PVCViewer${NC}"
    exit 0
fi

# 遍歷每個 namespace
for ns in $namespaces; do
    echo ""
    echo -e "${BLUE}Namespace: $ns${NC}"
    echo "----------------------------------------"
    
    # 獲取該 namespace 的所有 PVCViewer
    pvcviewer_names=$(echo "$pvcviewers" | jq -r ".items[] | select(.metadata.namespace==\"$ns\") | .metadata.name")
    
    for name in $pvcviewer_names; do
        total_pvcviewers=$((total_pvcviewers + 1))
        echo ""
        echo -e "${BLUE}檢查 PVCViewer: ${GREEN}$name${NC}"
        
        # 獲取 PVCViewer 詳細資訊
        pvcviewer=$(kubectl get pvcviewer "$name" -n "$ns" -o json)
        
        # 檢查是否有 Networking 配置
        has_networking=$(echo "$pvcviewer" | jq -r '.spec.networking != null and .spec.networking != {}')
        
        if [ "$has_networking" != "true" ]; then
            echo -e "  ${YELLOW}⊘ 未配置 Networking，跳過公開分享檢查${NC}"
            continue
        fi
        
        pvcviewers_with_networking=$((pvcviewers_with_networking + 1))
        echo -e "  ${GREEN}✓ 已配置 Networking${NC}"
        
        # 提取 basePrefix
        base_prefix=$(echo "$pvcviewer" | jq -r '.spec.networking.basePrefix // ""')
        if [ -z "$base_prefix" ]; then
            echo -e "  ${RED}✗ 警告: basePrefix 未設置${NC}"
        fi
        
        # 檢查各個資源
        all_ok=true
        
        # 1. 檢查 Finalizer
        echo -e "\n  ${BLUE}1. 檢查 Finalizer${NC}"
        finalizer=$(echo "$pvcviewer" | jq -r '.metadata.finalizers[]? | select(. == "pvcviewer.kubeflow.org/public-share-cleanup")')
        if [ -n "$finalizer" ]; then
            echo -e "     ${GREEN}✓ Finalizer 存在${NC}"
        else
            echo -e "     ${RED}✗ Finalizer 缺失${NC}"
            all_ok=false
        fi
        
        # 2. 檢查 VirtualService (share)
        echo -e "\n  ${BLUE}2. 檢查 Share VirtualService${NC}"
        vs_name="pvcviewer-share-$name"
        if kubectl get virtualservice "$vs_name" -n "$ns" &>/dev/null; then
            echo -e "     ${GREEN}✓ VirtualService 存在: $vs_name${NC}"
            
            # 驗證路徑配置
            vs_prefix=$(kubectl get virtualservice "$vs_name" -n "$ns" -o json | \
                jq -r '.spec.http[0].match[0].uri.prefix // ""')
            expected_prefix="$base_prefix/$ns/$name/share/"
            if [ "$vs_prefix" = "$expected_prefix" ]; then
                echo -e "     ${GREEN}✓ 路徑配置正確: $vs_prefix${NC}"
            else
                echo -e "     ${YELLOW}⚠ 路徑配置異常:${NC}"
                echo -e "       實際: $vs_prefix"
                echo -e "       預期: $expected_prefix"
            fi
        else
            echo -e "     ${RED}✗ VirtualService 不存在: $vs_name${NC}"
            all_ok=false
        fi
        
        # 3. 檢查 EnvoyFilter
        echo -e "\n  ${BLUE}3. 檢查 EnvoyFilter${NC}"
        ef_name="bypass-auth-pvcviewer-share-$ns-$name"
        if kubectl get envoyfilter "$ef_name" -n istio-system &>/dev/null; then
            echo -e "     ${GREEN}✓ EnvoyFilter 存在: $ef_name${NC}"
            
            # 檢查標籤
            ef_labels=$(kubectl get envoyfilter "$ef_name" -n istio-system -o json | \
                jq -r '.metadata.labels')
            pvcviewer_name_label=$(echo "$ef_labels" | jq -r '.["pvcviewer.kubeflow.org/name"] // ""')
            pvcviewer_ns_label=$(echo "$ef_labels" | jq -r '.["pvcviewer.kubeflow.org/namespace"] // ""')
            
            if [ "$pvcviewer_name_label" = "$name" ] && [ "$pvcviewer_ns_label" = "$ns" ]; then
                echo -e "     ${GREEN}✓ 追蹤標籤正確${NC}"
            else
                echo -e "     ${YELLOW}⚠ 追蹤標籤異常${NC}"
            fi
        else
            echo -e "     ${RED}✗ EnvoyFilter 不存在: $ef_name${NC}"
            all_ok=false
        fi
        
        # 4. 檢查 AuthorizationPolicy (Gateway)
        echo -e "\n  ${BLUE}4. 檢查 Gateway AuthorizationPolicy${NC}"
        ap_gw_name="allow-pvcviewer-share-$ns-$name-gw"
        if kubectl get authorizationpolicy "$ap_gw_name" -n istio-system &>/dev/null; then
            echo -e "     ${GREEN}✓ AuthorizationPolicy (Gateway) 存在: $ap_gw_name${NC}"
            
            # 檢查路徑
            ap_path=$(kubectl get authorizationpolicy "$ap_gw_name" -n istio-system -o json | \
                jq -r '.spec.rules[0].to[0].operation.paths[0] // ""')
            expected_path="$base_prefix/$ns/$name/share/*"
            if [ "$ap_path" = "$expected_path" ]; then
                echo -e "     ${GREEN}✓ 路徑配置正確: $ap_path${NC}"
            else
                echo -e "     ${YELLOW}⚠ 路徑配置異常:${NC}"
                echo -e "       實際: $ap_path"
                echo -e "       預期: $expected_path"
            fi
        else
            echo -e "     ${RED}✗ AuthorizationPolicy (Gateway) 不存在: $ap_gw_name${NC}"
            all_ok=false
        fi
        
        # 5. 檢查 AuthorizationPolicy (Backend)
        echo -e "\n  ${BLUE}5. 檢查 Backend AuthorizationPolicy${NC}"
        ap_backend_name="allow-pvcviewer-share-$name-inbound"
        if kubectl get authorizationpolicy "$ap_backend_name" -n "$ns" &>/dev/null; then
            echo -e "     ${GREEN}✓ AuthorizationPolicy (Backend) 存在: $ap_backend_name${NC}"
            
            # 檢查 HTTP 方法
            methods=$(kubectl get authorizationpolicy "$ap_backend_name" -n "$ns" -o json | \
                jq -r '.spec.rules[0].to[0].operation.methods[]?' | tr '\n' ',' | sed 's/,$//')
            expected_methods="GET,HEAD,OPTIONS"
            if [ "$methods" = "$expected_methods" ]; then
                echo -e "     ${GREEN}✓ HTTP 方法限制正確: $methods${NC}"
            else
                echo -e "     ${YELLOW}⚠ HTTP 方法異常:${NC}"
                echo -e "       實際: $methods"
                echo -e "       預期: $expected_methods"
            fi
            
            # 檢查路徑（應該包含兩個路徑）
            paths=$(kubectl get authorizationpolicy "$ap_backend_name" -n "$ns" -o json | \
                jq -r '.spec.rules[0].to[0].operation.paths[]?' | wc -l)
            if [ "$paths" = "2" ]; then
                echo -e "     ${GREEN}✓ 路徑配置數量正確（2個）${NC}"
            else
                echo -e "     ${YELLOW}⚠ 路徑配置數量異常: $paths (預期: 2)${NC}"
            fi
        else
            echo -e "     ${RED}✗ AuthorizationPolicy (Backend) 不存在: $ap_backend_name${NC}"
            all_ok=false
        fi
        
        # 總結
        echo ""
        if [ "$all_ok" = true ]; then
            echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
            echo -e "  ${GREEN}✓ 所有公開分享資源配置正確${NC}"
            echo -e "  ${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
            correct_resources=$((correct_resources + 1))
        else
            echo -e "  ${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
            echo -e "  ${RED}✗ 發現缺失或異常的資源${NC}"
            echo -e "  ${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
            missing_resources=$((missing_resources + 1))
        fi
    done
done

# 最終總結
echo ""
echo ""
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}檢查總結${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "總 PVCViewer 數量:            ${BLUE}$total_pvcviewers${NC}"
echo -e "配置 Networking 的數量:       ${BLUE}$pvcviewers_with_networking${NC}"
echo -e "公開分享資源完整的數量:       ${GREEN}$correct_resources${NC}"
echo -e "公開分享資源缺失的數量:       ${RED}$missing_resources${NC}"
echo ""

if [ $missing_resources -gt 0 ]; then
    echo -e "${YELLOW}建議操作:${NC}"
    echo -e "1. 對於缺失資源的 PVCViewer，觸發重新協調："
    echo -e "   ${BLUE}kubectl annotate pvcviewer <name> -n <namespace> reconcile=\$(date +%s)${NC}"
    echo ""
    echo -e "2. 檢查 Controller 日誌："
    echo -e "   ${BLUE}kubectl logs -n kubeflow deployment/pvcviewer-controller-manager -c manager --tail=100${NC}"
    echo ""
    echo -e "3. 如果問題持續，檢查 RBAC 權限："
    echo -e "   ${BLUE}kubectl get clusterrole pvcviewer-controller-role -o yaml | grep -A 5 'envoyfilters\\|authorizationpolicies'${NC}"
    echo ""
    exit 1
else
    echo -e "${GREEN}✓ 所有 PVCViewer 的公開分享資源都配置正確！${NC}"
    echo ""
    exit 0
fi