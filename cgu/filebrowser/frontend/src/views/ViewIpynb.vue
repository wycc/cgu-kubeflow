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
import { useRoute } from 'vue-router';
import { Notebook } from 'render-jupyter-notebook-vue/lib/Notebook/index.umd';
import { useFileStore } from "@/stores/file";
import { useLayoutStore } from "@/stores/layout";
import { useUploadStore } from "@/stores/upload";
import markdown from 'markdown-it';
import { pub as api } from "@/api";
import { Base64 } from "js-base64";

const fileStore = useFileStore();
const route = useRoute();
const notebookName = (route.query.file as string) || '';
const token = (route.query.token as string) || '';

const notebookContainer = ref<HTMLElement | null>(null);
const notebookContent = ref<any>(null);
let targetNamespace = "";
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
  targetNamespace = "pattentest"; // 先hardcode，之後要改成動態取得
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
  const url = `${base}/notebook/${targetNamespace}/editor/api/contents`;
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
// 若沒有 xsrf cookie，先打一次 lab base 讓伺服器種 cookie
async function ensureXsrfCookie(): Promise<void> {
  const before = document.cookie;
  const had = /xsrf/i.test(before);
  if (had) return;
  try {
    await fetch(labBaseUrl.value, { method: 'GET', credentials: 'include' });
  } catch (e) {
    console.warn('Preflight lab GET failed (still continuing):', e);
  }
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

async function getXsrfViaIframe(): Promise<string | null> {
  try {
    const iframe = await createHiddenIframe(labBaseUrl.value);
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
const sendPutRequest = async () => {
  const parts = window.location.href.split("/");
  // 先嘗試確保 xsrf cookie 存在（同網域下 server 會種 cookie）
  await ensureXsrfCookie();
  // 嘗試讀取多種可能的 xsrf cookie 名稱
  const xsrfToken = getCookie("_xsrf") || getCookie("XSRF-TOKEN") || getAnyXsrfCookie();
  let finalXsrf = xsrfToken;
  if (!finalXsrf) {
    // 若目前頁面路徑看不到 cookie，改用 iframe 在 lab 路徑下讀
    finalXsrf = await getXsrfViaIframe();
  }
  // 構建必要的 headers，包含 XSRF 與（若存在）分享 token 的授權
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    "X-Requested-With": "XMLHttpRequest",
  };
  if (finalXsrf) {
    headers["X-XSRFToken"] = finalXsrf;
  }
  if (token) {
    headers["Authorization"] = `token ${token}`;
  }
  // 如伺服器要求以 query 帶 token，可作為後備（保留原行為，僅在 xsrf 缺失且有 token 時嘗試）
  const targetUrl = token
    ? (jupyterUrl.value + "/" + notebookName + `?token=${encodeURIComponent(token)}`)
    : (jupyterUrl.value + "/" + notebookName);
  // 一律帶上 cookie，並同時送出 XSRF header（以及可能的 token）
  const credentialsMode: RequestCredentials = 'include';
  const response = await fetch(targetUrl, {
    method: "PUT",
    headers,
    credentials: credentialsMode,
    body: JSON.stringify({
      content: Base64.encode(notebookContent.value),
      format: "base64",
      name: notebookName,
      path: notebookName,
      type: "file",
    }),
  });
  if (response.ok){
    window.location.href = labBaseUrl.value + "/tree/" + notebookName;
  }
  else if (response.status === 503) {
    alert("Notebook is not running. Please start it first.");
  }
  else{
    alert("Unkown Error!!!")
  }
  
};
</script>