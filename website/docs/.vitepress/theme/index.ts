import DefaultTheme from 'vitepress/theme'
import type { Theme } from 'vitepress'
import './style/vars.css'
import './style/custom.css'
import HomeLayout from './components/HomeLayout.vue'
import HeroTerminal from './components/HeroTerminal.vue'
import InstallCommand from './components/InstallCommand.vue'
import FeatureCard from './components/FeatureCard.vue'
import FeatureGrid from './components/FeatureGrid.vue'
import CommandShowcase from './components/CommandShowcase.vue'
import DirectoryTree from './components/DirectoryTree.vue'
import StatusBadge from './components/StatusBadge.vue'
import HowItWorks from './components/HowItWorks.vue'

export default {
  extends: DefaultTheme,
  Layout: HomeLayout,
  enhanceApp({ app }) {
    app.component('HeroTerminal', HeroTerminal)
    app.component('InstallCommand', InstallCommand)
    app.component('FeatureCard', FeatureCard)
    app.component('FeatureGrid', FeatureGrid)
    app.component('CommandShowcase', CommandShowcase)
    app.component('DirectoryTree', DirectoryTree)
    app.component('StatusBadge', StatusBadge)
    app.component('HowItWorks', HowItWorks)
  },
} satisfies Theme
