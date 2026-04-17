"use client";

import { ShaderGradientCanvas, ShaderGradient } from "@shadergradient/react";

export default function ShaderGradientBg() {
  return (
    <div className="absolute inset-0 -z-10">
      <ShaderGradientCanvas
        style={{ width: "100%", height: "100%" }}
        pixelDensity={1}
        fov={45}
      >
        <ShaderGradient
          type="plane"
          animate="on"
          uSpeed={0.3}
          uStrength={3}
          uDensity={1.8}
          uFrequency={5.5}
          uAmplitude={1.5}
          positionX={-0.5}
          positionY={0}
          positionZ={0}
          rotationX={0}
          rotationY={10}
          rotationZ={50}
          color1="#5eead4"
          color2="#06b6d4"
          color3="#0f172a"
          reflection={0.1}
          brightness={1.4}
          grain="on"
          lightType="3d"
          envPreset="city"
          cAzimuthAngle={180}
          cPolarAngle={90}
          cDistance={3.6}
          cameraZoom={1}
        />
      </ShaderGradientCanvas>
      {/* Top fade so nav text is readable */}
      <div
        className="absolute inset-x-0 top-0 h-32"
        style={{
          background:
            "linear-gradient(to bottom, rgba(10,10,15,0.8), transparent)",
        }}
      />
      {/* Radial vignette: dark center for text readability, vivid edges */}
      <div
        className="absolute inset-0"
        style={{
          background:
            "radial-gradient(ellipse 55% 45% at 50% 42%, rgba(10,10,15,0.88) 0%, rgba(10,10,15,0.5) 55%, rgba(10,10,15,0.1) 100%)",
        }}
      />
      {/* Bottom fade to page background */}
      <div
        className="absolute inset-x-0 bottom-0 h-[75vh]"
        style={{
          background:
            "linear-gradient(to bottom, transparent 0%, rgba(10,10,15,0.15) 25%, rgba(10,10,15,0.45) 50%, rgba(10,10,15,0.8) 75%, #0a0a0f 100%)",
        }}
      />
    </div>
  );
}
