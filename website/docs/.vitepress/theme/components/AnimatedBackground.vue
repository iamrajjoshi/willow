<template>
  <div class="bg-canvas" aria-hidden="true">
    <div class="bg-orb orb-cyan"></div>
    <div class="bg-orb orb-magenta"></div>
    <div class="bg-orb orb-blue"></div>
    <div class="bg-orb orb-green"></div>
    <div class="bg-orb orb-yellow"></div>
    <div class="bg-noise"></div>
  </div>
</template>

<style scoped>
.bg-canvas {
  position: fixed;
  inset: 0;
  z-index: -1;
  overflow: hidden;
  pointer-events: none;
}

.bg-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(100px);
  will-change: transform;
}

.orb-cyan {
  width: 600px;
  height: 600px;
  background: rgba(139, 233, 253, 0.07);
  top: -10%;
  left: 15%;
  animation: drift-1 25s ease-in-out infinite;
}

.orb-magenta {
  width: 500px;
  height: 500px;
  background: rgba(252, 76, 180, 0.06);
  top: 10%;
  right: -5%;
  animation: drift-2 30s ease-in-out infinite;
}

.orb-blue {
  width: 700px;
  height: 700px;
  background: rgba(73, 186, 255, 0.05);
  bottom: -15%;
  left: 40%;
  animation: drift-3 35s ease-in-out infinite;
}

.orb-green {
  width: 400px;
  height: 400px;
  background: rgba(80, 251, 123, 0.05);
  top: 50%;
  left: -5%;
  animation: drift-4 28s ease-in-out infinite;
}

.orb-yellow {
  width: 350px;
  height: 350px;
  background: rgba(240, 251, 140, 0.03);
  top: 30%;
  right: 20%;
  animation: drift-5 32s ease-in-out infinite;
}

/* Each orb drifts on its own path */
@keyframes drift-1 {
  0%, 100% { transform: translate(0, 0) scale(1); }
  25% { transform: translate(80px, 60px) scale(1.1); }
  50% { transform: translate(-40px, 120px) scale(0.95); }
  75% { transform: translate(60px, -30px) scale(1.05); }
}

@keyframes drift-2 {
  0%, 100% { transform: translate(0, 0) scale(1); }
  25% { transform: translate(-70px, 80px) scale(1.05); }
  50% { transform: translate(50px, -40px) scale(1.1); }
  75% { transform: translate(-90px, -60px) scale(0.9); }
}

@keyframes drift-3 {
  0%, 100% { transform: translate(0, 0) scale(1); }
  33% { transform: translate(100px, -80px) scale(1.08); }
  66% { transform: translate(-60px, 50px) scale(0.92); }
}

@keyframes drift-4 {
  0%, 100% { transform: translate(0, 0) scale(1); }
  20% { transform: translate(50px, -70px) scale(1.12); }
  40% { transform: translate(90px, 30px) scale(0.95); }
  60% { transform: translate(-20px, 80px) scale(1.05); }
  80% { transform: translate(-60px, -40px) scale(0.98); }
}

@keyframes drift-5 {
  0%, 100% { transform: translate(0, 0) scale(1); }
  30% { transform: translate(-50px, 60px) scale(1.1); }
  60% { transform: translate(70px, -30px) scale(0.9); }
}

/* Film grain overlay via SVG noise */
.bg-noise {
  position: absolute;
  inset: 0;
  opacity: 0.035;
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
  background-repeat: repeat;
  background-size: 256px 256px;
}

/* Reduce motion for accessibility */
@media (prefers-reduced-motion: reduce) {
  .bg-orb {
    animation: none !important;
  }
}
</style>
