<template>
  <div class="breadcrumbs">
    <component
      :is="element"
      :to="base || ''"
      :aria-label="t('files.home')"
      :title="t('files.home')"
    >
      <i class="material-icons">home</i>
    </component>

    <span v-for="(link, index) in items" :key="index">
      <span class="chevron"
        ><i class="material-icons">keyboard_arrow_right</i></span
      >
      <component :is="element" :to="link.url">{{ link.name }}</component>
    </span>
    <button v-if="showEdit" @click="sendPutRequest">Edit</button>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute } from "vue-router";

const { t } = useI18n();

const route = useRoute();

const props = defineProps<{
  base: string;
  noLink?: boolean;
  showEdit?: boolean;
  name?: string;
  content?: string;
}>();

const items = computed(() => {
  const relativePath = route.path.replace(props.base, "");
  const parts = relativePath.split("/");

  if (parts[0] === "") {
    parts.shift();
  }

  if (parts[parts.length - 1] === "") {
    parts.pop();
  }

  const breadcrumbs: BreadCrumb[] = [];

  for (let i = 0; i < parts.length; i++) {
    if (i === 0) {
      breadcrumbs.push({
        name: decodeURIComponent(parts[i]),
        url: props.base + "/" + parts[i] + "/",
      });
    } else {
      breadcrumbs.push({
        name: decodeURIComponent(parts[i]),
        url: breadcrumbs[i - 1].url + parts[i] + "/",
      });
    }
  }

  if (breadcrumbs.length > 3) {
    while (breadcrumbs.length !== 4) {
      breadcrumbs.shift();
    }

    breadcrumbs[0].name = "...";
  }
  return breadcrumbs;
});

const element = computed(() => {
  if (props.noLink) {
    return "span";
  }

  return "router-link";
});

const fileUrl = computed(() => {
  const parts = window.location.href.split("/");
  console.log(parts)
  return parts.slice(0, 6).join("/") + "/api/public/file/";
});
const jupyterUrl = computed(() => {
  const parts = window.location.href.split("/");
  console.log(parts)
  if (parts.length > 0) {
    parts[3] = "notebook";
    parts[5] = "editor";
    parts[6] = "api";
    parts[7] = "contents";
  }
  return parts.slice(0, 8).join("/");
});


const sendPutRequest = async () => {
  const path = window.location.href.split("/").slice(7, -1).join("/");
  // const response = await fetch(fileUrl.value, {
  //   method: "PUT",
  //   headers: {
  //     "Content-Type": "application/json",
  //   },
  //   credentials: "include",
  //   body: JSON.stringify({
  //     url: jupyterUrl.value + "/" + props.name,
  //     type: "file",
  //     name: props.name,
  //     path: "/home/jovyan/",
  //     content: String(props.content),
  //   }),
  // });
  // console.log(await response.text());
  window.location.href = jupyterUrl.value.split("/").slice(0, 4).join("/") + "/" + window.location.href.split("/").slice(4, 6).join("/") + "/lab/tree/" + props.name;
};

</script>

<style></style>
