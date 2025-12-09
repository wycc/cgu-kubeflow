<!--This file is for the ipynb viewer in share mode-->

<template>
  <div>
    <button @click="sendPutRequest">Edit</button>
    <div ref="notebookContainer"></div>
  </div>
</template>

<script setup lang="ts">
import {
computed,
defineAsyncComponent,
onMounted,
onUnmounted,
ref,
watch,
nextTick,
} from "vue";
import { useRoute, useRouter } from 'vue-router';
import { Notebook } from 'render-jupyter-notebook-vue/lib/Notebook/index.umd';
import { useFileStore } from "@/stores/file";
import { useLayoutStore } from "@/stores/layout";
import { useUploadStore } from "@/stores/upload";
import markdown from 'markdown-it';
import { pub as api } from "@/api";
import { Base64 } from "js-base64";

const fileStore = useFileStore();
const route = useRoute();
const router = useRouter();
const notebookName = (route.query.file as string) || '';
const token = (route.query.token as string) || '';

const notebookContainer = ref<HTMLElement | null>(null);
const notebookContent = ref<any>(null);
let targetNamespace = ref<string | null>(null);
// 在載入時先準備好 XSRF token（非 reactive，供後續多個 API 使用）
let preloadedXsrf: string | null = null;
const defaultMarkdownParser = markdown({ // define a default markdown parser
  html: true,
  xhtmlOut: true,
  breaks: true,
  linkify: true,
});
const defaultMathJaxTypesetterConfig = { // define a default MathJax typesetter config
  url: 'https://cdnjs.cloudflare.com/ajax/libs/mathjax/2.7.7/MathJax.js',
  config: 'TeX-AMS_HTML-full,Safe',
};

onMounted(async () => {
  if (!notebookName) {
    console.error('No notebook specified');
    return;
  }

  // 取得 token
  const token = route.query.token as string || '';
  // 預先初始化 XSRF token（非阻塞，強制透過 iframe 取得）
  (async () => {
    try {
      preloadedXsrf = await getXsrfViaIframe(labBaseUrl.value);
    } catch (e) {
      console.warn('Preload XSRF failed:', e);
    }
  })();
  // 預先取得namespace
  (async () => {
    try {
      targetNamespace.value = await getNamespace();
    } catch (e) {
      console.warn('Preload namespace failed:', e);
    }
  })();
  try {
    // 取得 notebook 內容（用 /share/dl）
    const res = await fetch(`${window.location.href.split("/").slice(0, -2).join("/")}/share/dl/${encodeURIComponent(token)}?inline=true`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const text = await res.text();
    notebookContent.value = text;
  } catch (e) {
    console.error('Failed to fetch notebook:', e);
    return;
  }

  // 渲染 notebook
  if (notebookContainer.value && notebookContent.value) {
    const notebookInstance = new Notebook(
      JSON.parse(notebookContent.value),
      true,
      true,
      defaultMarkdownParser,
      defaultMathJaxTypesetterConfig
    );
    await notebookInstance.render();
    notebookContainer.value.innerHTML = "";
    notebookContainer.value.appendChild(notebookInstance.notebookHTML);
  } else {
    console.error('Content is not loaded');
  }

  const namespaceResponse = await fetch(window.location.href.split("/").slice(0, 3).join("/") + "/api/workgroup/exists", {
    method: "GET",
    credentials: "include"
  });
});


const fileUrl = computed(() => {
  const parts = window.location.href.split("/");
  const url = parts.slice(0, 6).join("/") + "/api/public/file/";
  return url;
});
const jupyterUrl = computed(() => {
  const parts = window.location.href.split("/");
  // protocol + host
  const base = parts.slice(0, 3).join("/");
  const url = `${base}/notebook/${targetNamespace.value}/editor/api/contents`;
  return url;
});
// 嘗試取得對應的 JupyterLab base URL（用於預先種下 xsrf cookie）
const labBaseUrl = computed(() => {
  const parts = jupyterUrl.value.split("/");
  // .../notebook/<ns>/editor/api/contents -> 取到 .../notebook/<ns>/editor
  const base = parts.slice(0, parts.indexOf('api')).join("/");
  const url = `${base}/lab`;
  return url;
});
// 讀取 cookie 的小工具
function getCookie(name: string): string | null {
  const cookieStr = document.cookie;
  if (!cookieStr) return null;
  const cookies = cookieStr.split(';');
  for (const c of cookies) {
    const [k, v] = c.trim().split('=');
    if (k === name) return decodeURIComponent(v || '');
  }
  return null;
}
// 擴充：嘗試尋找任何包含 xsrf 字樣的 cookie 名稱
function getAnyXsrfCookie(): string | null {
  const cookieStr = document.cookie;
  if (!cookieStr) return null;
  const cookies = cookieStr.split(';');
  for (const c of cookies) {
    const [k, v] = c.trim().split('=');
    if (!k) continue;
    if (/xsrf/i.test(k)) return decodeURIComponent(v || '');
  }
  return null;
}

// 建立一個隱藏 iframe 指向 labBaseUrl，讀取該路徑可見的 cookie（純前端做法）
function createHiddenIframe(src: string): Promise<HTMLIFrameElement> {
  return new Promise((resolve, reject) => {
    const iframe = document.createElement('iframe');
    iframe.src = src;
    iframe.style.display = 'none';
    iframe.onload = () => resolve(iframe);
    iframe.onerror = (err) => reject(err);
    document.body.appendChild(iframe);
  });
}

function parseXsrfFromCookieStr(cookieStr: string): string | null {
  if (!cookieStr) return null;
  const cookies = cookieStr.split(';');
  for (const c of cookies) {
    const [k, v] = c.trim().split('=');
    if (!k) continue;
    if (k === '_xsrf' || k === 'XSRF-TOKEN' || /xsrf/i.test(k)) {
      return decodeURIComponent(v || '');
    }
  }
  return null;
}

async function getXsrfViaIframe(url: string): Promise<string | null> {
  try {
    const iframe = await createHiddenIframe(url);
    // 等待一小段時間，確保 cookie 已被寫入
    await new Promise((r) => setTimeout(r, 100));
    const iframeDoc = iframe.contentDocument || iframe.ownerDocument;
    const cookieStr = iframeDoc ? iframeDoc.cookie : '';
    document.body.removeChild(iframe);
    const token = parseXsrfFromCookieStr(cookieStr);
    return token;
  } catch (e) {
    console.warn('Hidden iframe approach failed:', e);
    return null;
  }
}
async function getNamespace(){
  let finalXsrf = preloadedXsrf || await getXsrfViaIframe(labBaseUrl.value);
  if (!finalXsrf) finalXsrf = await getXsrfViaIframe(labBaseUrl.value);
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Requested-With': 'XMLHttpRequest',
  };
  if (finalXsrf) {
    headers['X-XSRFToken'] = finalXsrf;
  }
  if (token) {
    headers['Authorization'] = `token ${token}`;
  }
  const response = await fetch(
    window.location.href.split("/").slice(0, 3).join("/") + "/api/workgroup/env-info", {
      method: "GET",
      headers,
      credentials: "include"
  });
  let responseJson = await JSON.parse(await response.text())
  for (var i=0; i < responseJson.namespaces.length; i++){
    if (responseJson.namespaces[i].role === "owner"){
      return responseJson.namespaces[i].namespace;
    }
  }
}

async function startEditor(namespace: string | null){
  let finalXsrf = preloadedXsrf || await getXsrfViaIframe(window.location.href.split("/").slice(0, 3).join("/") + "/jupyter/api/");
  if (!finalXsrf) finalXsrf = await getXsrfViaIframe(window.location.href.split("/").slice(0, 3).join("/") + "/jupyter/api/");
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Requested-With': 'XMLHttpRequest',
  };
  if (finalXsrf) {
    headers['X-XSRF-TOKEN'] = finalXsrf;
  }
  if (token) {
    headers['Authorization'] = `token ${token}`;
  }
  const patchBody = {
    "stopped":false
  }
  const response = await fetch(
    window.location.href.split("/").slice(0, 3).join("/") + `/jupyter/api/namespaces/${namespace}/notebooks/editor`, {
      method: "PATCH",
      headers,
      credentials: "include",
      body: JSON.stringify(patchBody)
  });
  let responseJson = await JSON.parse(await response.text())
  console.log(response.text())
}
const sendPutRequest = async () => {
  if (!targetNamespace.value) targetNamespace.value = await getNamespace();
  const parts = window.location.href.split("/");
  // 一律透過 iframe 取得/刷新 xsrf（避免不同路徑導致 cookie 不可見）
  let finalXsrf = preloadedXsrf;
  if (!finalXsrf) finalXsrf = await getXsrfViaIframe(labBaseUrl.value);
  // 構建必要的 headers，包含 XSRF 與（若存在）分享 token 的授權
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Requested-With': 'XMLHttpRequest',
  };
  if (finalXsrf) {
    headers['X-XSRFToken'] = finalXsrf;
  }
  if (token) {
    headers['Authorization'] = `token ${token}`;
  }
  // 如伺服器要求以 query 帶 token，可作為後備（保留原行為，僅在 xsrf 缺失且有 token 時嘗試）
  const targetUrl = token
    ? (jupyterUrl.value + "/" + notebookName + `?token=${encodeURIComponent(token)}`)
    : (jupyterUrl.value + "/" + notebookName);
  // 一律帶上 cookie，並同時送出 XSRF header（以及可能的 token）
  // const credentialsMode: RequestCredentials = 'include';
  const response = await fetch(targetUrl, {
    method: "PUT",
    headers,
    credentials: "include",
    body: JSON.stringify({
      content: Base64.encode(notebookContent.value),
      format: "base64",
      name: notebookName,
      path: notebookName,
      type: "file",
    }),
  });
  // if (response.ok){
  //   window.location.href = labBaseUrl.value + "/tree/" + notebookName;
  // }
  // else if (response.status === 503) {
    // Notebook not running: open the app's wait/editor page so user can start/edit it.
  console.log('PUT returned 503 — attempting to navigate to /share/waitNotebook', { notebookName, token, namespace: targetNamespace.value });

  const routeLocation = {
    path: '/share/waitNotebook',
    query: {
      file: notebookName,
      token: token,
      namespace: targetNamespace.value || undefined,
    },
  };

  try {
    // Directly set window.location.href to avoid router permission issues
    const resolved = router.resolve({ path: '/share/waitNotebook', query: { file: notebookName, token, namespace: targetNamespace.value || undefined } });
    console.log('Navigating to /share/waitNotebook via window.location.href', resolved.href);
    window.location.href = resolved.href;
  } catch (e) {
    console.error('Failed to build wait URL, falling back to location-based path:', e);
    // best-effort fallback using origin
    const origin = window.location.origin || window.location.href.split('/').slice(0,3).join('/');
    const qs = new URLSearchParams({ file: notebookName, token: token });
    if (targetNamespace.value) qs.set('namespace', targetNamespace.value);
    window.location.href = `${origin}/share/wait?${qs.toString()}`;
  }

  // Also attempt to start the editor runtime on the backend
  startEditor(targetNamespace.value);
  // }
  // else{
  //   alert("Unkown Error!!!")
  // }
  
};
</script>