<template>
  <div>
    <p>{{ message }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { useRoute } from 'vue-router';
import { Base64 } from 'js-base64';
import { useFileStore } from "@/stores/file";
import { useLayoutStore } from "@/stores/layout";
import { useUploadStore } from "@/stores/upload";
import { pub as api } from "@/api";
import { useI18n } from "vue-i18n";


const route = useRoute();
const notebookName = (route.query.file as string) || '';
const token = (route.query.token as string) || '';
let targetNamespace = (route.query.namespace as string) || null;

const message = ref('Waiting for notebook to start...');
const pollIntervalMs = 3000;
let pollTimer: number | null = null;
let notebookContent: string | null = null;

// --- XSRF helpers (copied/adapted from ViewIpynb.vue) ---
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

async function getXsrfViaIframe(url: string): Promise<string | null> {
  try {
    const iframe = await createHiddenIframe(url);
    // wait briefly to let cookies be set
    await new Promise((r) => setTimeout(r, 100));
    const iframeDoc = (iframe.contentDocument as Document) || (iframe.ownerDocument as Document) || null;
    const cookieStr = iframeDoc ? iframeDoc.cookie : '';
    document.body.removeChild(iframe);
    const token = parseXsrfFromCookieStr(cookieStr);
    return token;
  } catch (e) {
    console.warn('Hidden iframe approach failed:', e);
    return null;
  }
}

async function fetchNotebookContent() {
  if (!token) return null;
  try {
    const res = await fetch(`${window.location.href.split('/').slice(0, -2).join('/')}/share/dl/${encodeURIComponent(token)}?inline=true`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const text = await res.text();
    notebookContent = text;
    return text;
  } catch (e) {
    console.error('Failed to fetch notebook content:', e);
    return null;
  }
}

async function getNamespaceFromEnv(): Promise<string | null> {
  try {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'X-Requested-With': 'XMLHttpRequest',
    };
    const resp = await fetch(window.location.href.split('/').slice(0, 3).join('/') + '/api/workgroup/env-info', {
      method: 'GET',
      headers,
      credentials: 'include'
    });
    if (!resp.ok) return null;
    const data = await resp.json();
    for (let i = 0; i < (data.namespaces || []).length; i++) {
      if (data.namespaces[i].role === 'owner') return data.namespaces[i].namespace;
    }
    return null;
  } catch (e) {
    console.warn('getNamespaceFromEnv failed', e);
    return null;
  }
}

async function sendPutRequest() {
  if (!targetNamespace) targetNamespace = await getNamespaceFromEnv();

  const base = window.location.href.split("/").slice(0, 3).join("/");
  const jupyterUrl = `${base}/notebook/${targetNamespace || ''}/editor/api/contents`;
  const labBase = jupyterUrl.split('/').slice(0, jupyterUrl.split('/').indexOf('api')).join('/') + '/lab';

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Requested-With': 'XMLHttpRequest',
  };
  // Always obtain XSRF token via hidden iframe (do not rely on any query param)
  let finalXsrfToken: string | null = null;
  try {
    // try the jupyter-prefixed path first, then fallback to app root
    finalXsrfToken = await getXsrfViaIframe(labBase)
  } catch (e) {
    console.warn('getXsrfViaIframe failed', e);
  }
  if (finalXsrfToken) {
    headers['X-XSRFToken'] = finalXsrfToken;
  }

  if (token) {
    headers['Authorization'] = `token ${token}`;
  }

  const targetUrl = token ? (jupyterUrl + '/' + notebookName + `?token=${encodeURIComponent(token)}`) : (jupyterUrl + '/' + notebookName);

  const body = JSON.stringify({
    content: Base64.encode(notebookContent || ''),
    format: 'base64',
    name: notebookName,
    path: notebookName,
    type: 'file',
  });

  try {
    const response = await fetch(targetUrl, {
      method: 'PUT',
      headers,
      credentials: 'include',
      body,
    });
    if (response.ok) {
      window.location.href = labBase + '/tree/' + notebookName;
      return true;
    }
    console.warn('PUT returned', response.status);
    return false;
  } catch (e) {
    console.error('sendPutRequest failed', e);
    return false;
  }
}

async function checkStatusOnce() {
  if (!targetNamespace) targetNamespace = await getNamespaceFromEnv();
  if (!targetNamespace || !notebookName) return;
  try {
    const resp = await fetch(`/jupyter/api/notebook/status?namespace=${encodeURIComponent(targetNamespace as string)}`, {
      method: 'GET',
      credentials: 'include',
      headers: { 'X-Requested-With': 'XMLHttpRequest' },
    });
    message.value = message.value + ".";
    if (!resp.ok) {
      console.warn('status endpoint returned', resp.status);
      return;
    }
    const j = await resp.json();
    const payload = j.notebook_status || {};
    if (payload.running) {
      message.value = 'Notebook is running. Uploading and redirecting...';
      if (await sendPutRequest()) {
        stopPolling();
      } else {
        message.value = 'Failed to upload. Retrying...';
      }
    }
  } catch (e) {
    console.error('checkStatusOnce error', e);
  }
}

function startPolling() {
  if (pollTimer) return;
  pollTimer = window.setInterval(checkStatusOnce, pollIntervalMs);
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

onMounted(async () => {
  if (!notebookName) {
    message.value = 'No notebook specified.';
    return;
  }
  await fetchNotebookContent();
  startPolling();
});

onUnmounted(() => {
  stopPolling();
});
</script>
