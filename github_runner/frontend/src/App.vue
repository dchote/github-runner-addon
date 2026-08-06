<template>
  <v-app>
    <v-app-bar class="app-header" color="primary" density="comfortable" flat>
      <v-app-bar-title class="text-title-medium font-weight-bold">
        GitHub Runner Manager
      </v-app-bar-title>
      <v-spacer />
      <v-chip
        v-if="dockerAvailable !== null"
        class="mr-2"
        size="small"
        label
        :color="dockerAvailable ? 'success' : 'error'"
        variant="flat"
        :prepend-icon="dockerAvailable ? undefined : 'mdi-alert'"
        :title="dockerError || undefined"
      >
        {{ dockerEngine }}
      </v-chip>
      <v-btn
        icon="mdi-cog-outline"
        variant="text"
        aria-label="Settings"
        @click="settingsOpen = true"
      />
    </v-app-bar>

    <v-main class="pa-0">
      <v-container class="page-content" style="max-width: 1100px">
        <router-view />
      </v-container>
    </v-main>

    <v-footer app class="app-footer" color="transparent" elevation="0">
      <a
        class="text-body-small app-footer__link"
        :href="docsHref"
        target="_blank"
        rel="noopener"
      >
        API Docs
      </a>
    </v-footer>

    <SettingsDialog v-model="settingsOpen" />
  </v-app>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useStore } from 'vuex'
import { resolveURL } from '@/utils/api'
import SettingsDialog from '@/components/SettingsDialog.vue'

// Health is loaded by RunnersPage (and poll); App only reads store for the Docker chip.
const store = useStore()
const settingsOpen = ref(false)
const dockerAvailable = computed(() => store.state.dockerAvailable)
const dockerError = computed(() => store.state.dockerError)
const dockerEngine = computed(() => store.state.dockerEngine || 'Docker')
const docsHref = computed(() => resolveURL('/docs'))
</script>

<style scoped>
.app-footer {
  justify-content: flex-start;
  padding-block: 8px !important;
  padding-inline: 16px !important;
  min-height: 0 !important;
  background: transparent !important;
}

.app-footer__link {
  color: var(--brand-muted) !important;
  text-decoration: none;
}

.app-footer__link:hover {
  color: rgb(var(--v-theme-primary)) !important;
  text-decoration: underline;
}
</style>
