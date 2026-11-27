plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.user.mywgclient"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.user.mywgclient"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
    buildFeatures {
        viewBinding = true
    }
    lint {
        disable += "GradleDependency"
    }
}

val wgmobileAar = layout.projectDirectory.file("libs/wgmobile.aar").asFile

dependencies {
    implementation(files(wgmobileAar))
    // These stay on the compileSdk 35 line. Newer AndroidX releases require
    // compileSdk 36+ and a newer Android Gradle Plugin.
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("com.google.android.material:material:1.12.0")
}

tasks.configureEach {
    if (name == "preBuild") {
        doFirst {
            if (!wgmobileAar.isFile) {
                throw GradleException(
                    "Missing ${wgmobileAar.path}. From the repo root run ./android/scripts/build_aar.sh " +
                        "(needs Go, gomobile, ANDROID_HOME, and ANDROID_NDK_HOME). See android/README.md.",
                )
            }
        }
    }
}
