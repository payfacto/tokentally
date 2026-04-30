import { createRouter, createWebHashHistory } from 'vue-router'
import OverviewView    from '../views/OverviewView.vue'
import PromptsView     from '../views/PromptsView.vue'
import SessionsView    from '../views/SessionsView.vue'
import ProjectsView    from '../views/ProjectsView.vue'
import SkillsView      from '../views/SkillsView.vue'
import TipsView        from '../views/TipsView.vue'
import SettingsView    from '../views/SettingsView.vue'
import CalculatorView  from '../views/CalculatorView.vue'
import OverageView     from '../views/OverageView.vue'

export default createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/',               redirect: '/overview' },
    { path: '/overview',       component: OverviewView },
    { path: '/prompts',        component: PromptsView },
    { path: '/sessions',       component: SessionsView },
    { path: '/sessions/:id',   component: SessionsView },
    { path: '/projects',       component: ProjectsView },
    { path: '/skills',         component: SkillsView },
    { path: '/tips',           component: TipsView },
    { path: '/tools',          component: OverageView },
    { path: '/settings',       component: SettingsView },
    { path: '/calculator',     component: CalculatorView },
  ],
})
