plugins {
    id("com.android.application")
}

android {
    namespace = "com.bowerbird.mobile"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.bowerbird.mobile.dev"
        minSdk = 28
        targetSdk = 36
        versionCode = 3
        versionName = "1.1.0-dev"
    }

    buildTypes {
        release {
            // Replace with a real keystore before publishing; debug signing is
            // only for local verification.
            signingConfig = signingConfigs.getByName("debug")
            isMinifyEnabled = false
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlin {
        compilerOptions { jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17) }
    }

    packaging {
        jniLibs { useLegacyPackaging = true }
    }
}

dependencies {
    implementation(files("../libs/Bowerbird.aar"))
    implementation("com.google.zxing:core:3.5.3")
    implementation("androidx.recyclerview:recyclerview:1.3.2")
    implementation("androidx.swiperefreshlayout:swiperefreshlayout:1.1.0")
}
