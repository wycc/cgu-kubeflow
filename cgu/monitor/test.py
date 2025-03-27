import requests
import datetime

prometheus_url = "http://<PROMETHEUS_SERVER>:9090/api/v1/query_range"

def query_prometheus(query, start, end, step="1h"):
    params = {
        "query": query,
        "start": start.isoformat(),
        "end": end.isoformat(),
        "step": step
    }
    response = requests.get(prometheus_url, params=params)
    data = response.json()
    if data["status"] != "success":
        raise Exception("Prometheus query failed: " + str(data))
    return data["data"]["result"]

# 範例：查詢一段期間內的 namespace_cpu_cost 數據
start_time = datetime.datetime(2025, 3, 1)
end_time = datetime.datetime(2025, 3, 31)
query = 'namespace_cpu_cost{exported_namespace="your-namespace"}'
result = query_prometheus(query, start_time, end_time)
