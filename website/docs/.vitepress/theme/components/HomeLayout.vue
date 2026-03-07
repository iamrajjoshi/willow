<script setup lang="ts">
import DefaultTheme from 'vitepress/theme'
import { useData } from 'vitepress'
import AnimatedBackground from './AnimatedBackground.vue'

const { Layout } = DefaultTheme
const { frontmatter } = useData()
</script>

<template>
  <AnimatedBackground v-if="frontmatter.layout === 'home'" />
  <Layout />
</template>
