import re
import time
# import threading
import os
from kubernetes import client, config
from prometheus_client import start_http_server, Gauge
from kubernetes.client.rest import ApiException
from kubernetes.client import CustomObjectsApi

# 加载 Kubernetes 配置
try:
    config.load_incluster_config()
except:
    config.load_kube_config()

# 初始化 Kubernetes API 客户端
v1 = client.CoreV1Api()
custom_api = client.CustomObjectsApi()

# 定义 Prometheus 指标
cpu_allocatable_metric = Gauge('node_cpu_allocatable', 'Allocatable CPU cores per node', ['node'])
gpu_allocatable_metric = Gauge('node_gpu_allocatable', 'Allocatable GPUs per node', ['node'])
memory_allocatable_metric = Gauge('node_memory_allocatable', 'Allocatable memory per node', ['node'])
node_usage_cpu_metric = Gauge('node_usage_cpu', 'CPU usage per node', ['node'])
node_usage_gpu_metric = Gauge('node_usage_gpu', 'GPU usage per node', ['node'])
namespace_usage_cpu_metric = Gauge('namespace_usage_cpu', 'CPU usage per namespace', ['namespace'])
namespace_usage_gpu_metric = Gauge('namespace_usage_gpu', 'GPU usage per namespace', ['namespace'])
notebook_usage_cpu = Gauge('notebook_cpu_usage', 'CPU usage of notebooks', ['node', 'namespace', 'notebook'])
notebook_usage_memory = Gauge('notebook_memory_usage', 'Memory usage of notebooks', ['node', 'namespace', 'notebook'])
notebook_usage_gpu = Gauge('notebook_gpu_usage', 'GPU usage of notebooks', ['node', 'namespace', 'notebook'])
namespace_cpu_cost_metric = Gauge('namespace_cpu_cost', 'Cumulative CPU cost per namespace', ['namespace'])
namespace_gpu_cost_metric = Gauge('namespace_gpu_cost', 'Cumulative GPU cost per namespace', ['namespace'])

# 定义配置常量
PROFILE_GROUP = "kubeflow.org"
PROFILE_VERSION = "v1"
PROFILE_PLURAL = "profiles"
CPU_COST_PER_MINUTE = float(os.environ.get("CPU_COST_PER_MINUTE", "1"))
GPU_COST_PER_MINUTE = float(os.environ.get("GPU_COST_PER_MINUTE", "2"))

# 上一次的缓存数据
previous_data = {
    'node_usage_cpu': {},
    'node_usage_gpu': {},
    'namespace_usage_cpu': {},
    'namespace_usage_gpu': {},
    'notebook_usage_cpu_data': {},
    'notebook_usage_gpu_data': {},
    'notebook_usage_memory_data': {},
    'namespace_cpu_cost': {},  # CPU 成本缓存
    'namespace_gpu_cost': {}   # GPU 成本缓存
}

def parse_memory_string(memory_str):
    """解析内存字符串为GB单位"""
    if memory_str.endswith('Ki'):
        return float(memory_str[:-2]) / (1024 * 1024)
    elif memory_str.endswith('Mi'):
        return float(memory_str[:-2]) / 1024.0
    elif memory_str.endswith('Gi'):
        return float(memory_str[:-2])
    elif memory_str.endswith('M'):
        return float(memory_str[:-1]) / 1024.0
    elif memory_str.endswith('K'):
        return float(memory_str[:-1]) / (1024 * 1024)
    elif memory_str.endswith('G'):
        return float(memory_str[:-1])
    else:
        return float(memory_str) 

def get_cpu_used(pod):
    """获取 Pod 的 CPU 使用量"""
    cpu_used = 0
    for container in pod.spec.containers:
        resources = container.resources
        if resources and resources.requests and 'cpu' in resources.requests:
            cpu_request = resources.requests['cpu']
            if cpu_request[-1] == 'm':
                cpu_used += int(cpu_request[:-1]) / 1000.0
            else:
                cpu_used += int(cpu_request)
    return cpu_used

def get_gpu_used(pod):
    """获取 Pod 的 GPU 使用量"""
    gpu_used = 0
    for container in pod.spec.containers:
        resources = container.resources
        if resources and resources.requests:
            for resource_name, resource_quantity in resources.requests.items():
                if 'gpu' in resource_name.lower():
                    gpu_used += int(resource_quantity)
    return gpu_used

def get_memory_used(pod):
    """获取 Pod 的内存使用量"""
    memory_used = 0
    for container in pod.spec.containers:
        resources = container.resources
        if resources and resources.requests and 'memory' in resources.requests:
            memory_request = resources.requests['memory']
            try:
                memory_used += parse_memory_string(memory_request)
            except ValueError:
                print(f"Unknown memory format: {memory_request}")
    return memory_used

def get_notebook_name(pod):
    """通过 labels 判断是否是 Notebook 生成的 Pod"""
    labels = pod.metadata.labels
    # 检查 notebook-name label
    notebook_name = labels.get("notebook-name")
    if notebook_name and pod.status.phase == "Running":
        return notebook_name
    return None
    # if pod.metadata.name:
    #     is_running = pod.status.phase == "Running"
    #     has_pattern = bool(re.search(r'-\d+$', pod.metadata.name))
    #     if has_pattern:
    #         print(f"Found notebook candidate: {pod.metadata.name}, running: {is_running}")
    #         if is_running:
    #             return pod.metadata.name
    # return None

def cleanup_previous_data(current, previous, gauge):
    """清理未更新的过期数据"""
    for key in list(previous.keys()):
        if key not in current:
            # 如果 key 是 tuple，检查是否有 None，并做替换
            if isinstance(key, tuple):
                labels = tuple(x if x is not None else "unknown" for x in key)
            else:
                labels = (key if key is not None else "unknown",)
            gauge.labels(*labels).set(0)
            del previous[key]

def get_existing_cost(namespace):
    """获取 namespace 现有的成本数据"""
    try:
        profile = custom_api.get_cluster_custom_object(
            group=PROFILE_GROUP,
            version=PROFILE_VERSION,
            plural=PROFILE_PLURAL,
            name=namespace
        )
        labels = profile.get("metadata", {}).get("labels", {})
        cpu_cost = float(labels.get("cpucost", "0"))
        gpu_cost = float(labels.get("gpucost", "0"))
        return cpu_cost, gpu_cost
    except Exception as e:
        print(f"Error reading existing cost for {namespace}: {e}")
        return 0, 0

def update_profile_labels(namespace_cpu_cost, namespace_gpu_cost):
    """更新 Profile 标签中的成本数据"""
    try:
        profiles = custom_api.list_cluster_custom_object(
            group=PROFILE_GROUP,
            version=PROFILE_VERSION,
            plural=PROFILE_PLURAL
        )

        for profile in profiles.get('items', []):
            metadata = profile.get("metadata", {})
            namespace = metadata.get("name")
            if not namespace:
                continue
            
            # 先读取existing成本数据
            existing_cpu, existing_gpu = get_existing_cost(namespace)
            # 获取本次计算的新增成本
            new_cpu = namespace_cpu_cost.get(namespace, 0)
            new_gpu = namespace_gpu_cost.get(namespace, 0)
            total_cpu = existing_cpu + new_cpu
            total_gpu = existing_gpu + new_gpu
            
            patch = {
                "metadata": {
                    "labels": {
                        "cpucost": f"{total_cpu:.2f}",
                        "gpucost": f"{total_gpu:.2f}"
                    }
                }
            }
            
            custom_api.patch_cluster_custom_object(
                group=PROFILE_GROUP,
                version=PROFILE_VERSION,
                plural=PROFILE_PLURAL,
                name=namespace,
                body=patch
            )
            print(f"Updated profile {namespace}: CPU={total_cpu:.2f}, GPU={total_gpu:.2f}")

    except ApiException as e:
        print(f"Profile update failed: {e.status} - {e.reason}")

def update_metrics():
    """更新所有指标"""
    global previous_data
    
    # 创建当前数据的副本，而不是引用
    current_data = {
        'node_usage_cpu': {},
        'node_usage_gpu': {},
        'namespace_usage_cpu': {},
        'namespace_usage_gpu': {},
        'notebook_usage_cpu_data': {},
        'notebook_usage_gpu_data': {},
        'notebook_usage_memory_data': {},
        'namespace_cpu_cost': {},
        'namespace_gpu_cost': {}
    }
    
    try:
        #-------------debugg
        nodes_response = v1.list_node()
        print(f"成功獲取節點列表，節點數量: {len(nodes_response.items) if hasattr(nodes_response, 'items') else '無items屬性'}")
        #-------------debugg
        # 获取现有的成本值
        profiles = custom_api.list_cluster_custom_object(
            group=PROFILE_GROUP,
            version=PROFILE_VERSION,
            plural=PROFILE_PLURAL
        )
        
        for profile in profiles.get('items', []):
            namespace = profile['metadata']['name']
            # 从 profile 标签中读取现有成本
            labels = profile.get("metadata", {}).get("labels", {})
            cpu_cost = float(labels.get("cpucost", "0"))
            gpu_cost = float(labels.get("gpucost", "0"))
            
            # 更新成本指标
            namespace_cpu_cost_metric.labels(namespace=namespace).set(cpu_cost)
            namespace_gpu_cost_metric.labels(namespace=namespace).set(gpu_cost)
            
        # 获取所有节点
        
        nodes = v1.list_node().items
        pods = v1.list_pod_for_all_namespaces().items
        namespaces = v1.list_namespace().items

        # 节点级别的资源可分配量
        for node in nodes:
            node_name = node.metadata.name
            cpu_allocatable = float(node.status.allocatable.get('cpu', '0'))
            gpu_allocatable = float(node.status.allocatable.get('nvidia.com/gpu', '0'))
            memory_allocatable = parse_memory_string(node.status.allocatable.get('memory', '0'))
            
            cpu_allocatable_metric.labels(node=node_name).set(cpu_allocatable)
            gpu_allocatable_metric.labels(node=node_name).set(gpu_allocatable)
            memory_allocatable_metric.labels(node=node_name).set(memory_allocatable)

            current_data['node_usage_cpu'][node_name] = 0
            current_data['node_usage_gpu'][node_name] = 0

        # 初始化命名空间使用量
        for namespace in namespaces:
            namespace_name = namespace.metadata.name
            current_data['namespace_usage_cpu'][namespace_name] = 0
            current_data['namespace_usage_gpu'][namespace_name] = 0

        # Pod 的资源使用量
        for pod in pods:
            node_name = pod.spec.node_name
            namespace_name = pod.metadata.namespace
            notebook_name = get_notebook_name(pod)  # 修改为只返回运行中的notebook名称

            if node_name and notebook_name: # Notebook Pod 时才统计用量
                cpu_allocated = get_cpu_used(pod)
                gpu_allocated = get_gpu_used(pod)
                memory_allocated = get_memory_used(pod)

                # 累加到 namespace & node 的资源使用量
                current_data['namespace_usage_cpu'][namespace_name] = current_data['namespace_usage_cpu'].get(namespace_name, 0) + cpu_allocated
                current_data['namespace_usage_gpu'][namespace_name] = current_data['namespace_usage_gpu'].get(namespace_name, 0) + gpu_allocated
                current_data['node_usage_cpu'][node_name] = current_data['node_usage_cpu'].get(node_name, 0) + cpu_allocated
                current_data['node_usage_gpu'][node_name] = current_data['node_usage_gpu'].get(node_name, 0) + gpu_allocated

                # 更新Notebook的资源使用量
                notebook_key = (node_name, namespace_name, notebook_name)
                current_data['notebook_usage_cpu_data'][notebook_key] = cpu_allocated
                current_data['notebook_usage_memory_data'][notebook_key] = memory_allocated
                current_data['notebook_usage_gpu_data'][notebook_key] = gpu_allocated

                # 更新Prometheus指标
                notebook_usage_cpu.labels(node=node_name, namespace=namespace_name, notebook=notebook_name).set(cpu_allocated)
                notebook_usage_memory.labels(node=node_name, namespace=namespace_name, notebook=notebook_name).set(memory_allocated)
                notebook_usage_gpu.labels(node=node_name, namespace=namespace_name, notebook=notebook_name).set(gpu_allocated)

        # 更新节点和命名空间的使用量指标
        for node_name, cpu_usage in current_data['node_usage_cpu'].items():
            node_usage_cpu_metric.labels(node=node_name).set(cpu_usage)
        for node_name, gpu_usage in current_data['node_usage_gpu'].items():
            node_usage_gpu_metric.labels(node=node_name).set(gpu_usage)
        
        for namespace_name, cpu_usage in current_data['namespace_usage_cpu'].items():
            namespace_usage_cpu_metric.labels(namespace=namespace_name).set(cpu_usage)
            # 更新CPU成本
            if cpu_usage > 0:
                current_data['namespace_cpu_cost'][namespace_name] = cpu_usage * CPU_COST_PER_MINUTE
  
        for namespace_name, gpu_usage in current_data['namespace_usage_gpu'].items():
            namespace_usage_gpu_metric.labels(namespace=namespace_name).set(gpu_usage)
            # 更新GPU成本
            if gpu_usage > 0:
                current_data['namespace_gpu_cost'][namespace_name] = gpu_usage * GPU_COST_PER_MINUTE

        # 清理过期数据
        cleanup_previous_data(current_data['node_usage_cpu'], previous_data['node_usage_cpu'], node_usage_cpu_metric)
        cleanup_previous_data(current_data['node_usage_gpu'], previous_data['node_usage_gpu'], node_usage_gpu_metric)
        cleanup_previous_data(current_data['namespace_usage_cpu'], previous_data['namespace_usage_cpu'], namespace_usage_cpu_metric)
        cleanup_previous_data(current_data['namespace_usage_gpu'], previous_data['namespace_usage_gpu'], namespace_usage_gpu_metric)
        cleanup_previous_data(current_data['notebook_usage_cpu_data'], previous_data['notebook_usage_cpu_data'], notebook_usage_cpu)
        cleanup_previous_data(current_data['notebook_usage_gpu_data'], previous_data['notebook_usage_gpu_data'], notebook_usage_gpu)
        cleanup_previous_data(current_data['notebook_usage_memory_data'], previous_data['notebook_usage_memory_data'], notebook_usage_memory)

        # 更新Profile标签
        update_profile_labels(current_data['namespace_cpu_cost'], current_data['namespace_gpu_cost'])
        
        # 正确更新全局变量
        previous_data.clear()
        previous_data.update(current_data)

    except ApiException as e:
        print(f"Kubernetes API Error: {e.status} - {e.reason}")
    except Exception as e:
        print(f"Unexpected error: {str(e)}")

if __name__ == "__main__":
    start_http_server(8080)
    while True:
        update_metrics()
        print("running")
        time.sleep(60)