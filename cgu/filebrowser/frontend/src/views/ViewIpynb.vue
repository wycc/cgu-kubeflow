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

const fileStore = useFileStore();
const route = useRoute();
const notebookName = (route.query.file as string) || '';
const token = (route.query.token as string) || '';

const notebookContainer = ref<HTMLElement | null>(null);
const notebookContent = ref<any>(null);
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
});


const fileUrl = computed(() => {
  const parts = window.location.href.split("/");
  const url = parts.slice(0, 6).join("/") + "/share/file/";
  console.log(url);
  return parts.slice(0, 6).join("/") + "/share/file/";
});
const jupyterUrl = computed(() => {
  const parts = window.location.href.split("/");
  const url = parts.slice(0, 6).join("/") + "/share" + "/api" + "/contents";
  console.log(url);
  if (parts.length > 0) {
    parts[3] = "notebook";
    parts[5] = "editor";
  }
  return parts.slice(0, 6).join("/") + "/share" + "/api" + "/contents";
});
const sendPutRequest = async () => {
  const parts = window.location.href.split("/");
  const response = await fetch(fileUrl.value, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    credentials: "include",
    body: JSON.stringify({
      url: jupyterUrl.value + "/" + notebookName,
      type: "file",
      name: notebookName,
      path: "/home/jovyan/",
      // content: btoa(unescape(encodeURIComponent(String(props.content)))),
      content: notebookContent.value,
    }),
  });
  console.log(await response.text());
  if (response.status == 200){
    window.location.href = jupyterUrl.value.split("/").slice(0, 4).join("/") + "/" + window.location.href.split("/")[4] + "/editor/lab/tree/" + notebookName;
  }
  else if (response.status == 202) {
    alert("Notebook is not running. Starting it, please retry later.");
  }
  
};
</script>