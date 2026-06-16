import { createApp } from 'vue'
import { createPinia } from 'pinia'
import './style.css'
import App from './App.vue'
import router from './router'
import VNetworkGraph from "v-network-graph"
import "v-network-graph/lib/style.css"

const app = createApp(App)
const pinia = createPinia()
app.use(router)
app.use(pinia)
app.use(VNetworkGraph)
app.mount('#app')
