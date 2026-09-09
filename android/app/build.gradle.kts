plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.ksp)
    alias(libs.plugins.hilt)
}

android {
    namespace = "com.edgewatcher"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.edgewatcher"
        minSdk = 26
        targetSdk = 36
        versionCode = 1
        versionName = "1.0.0"
        buildConfigField("String", "APP_VERSION", "\"1.0.0\"")
    }

    buildTypes {
        debug {
            buildConfigField(
                "String",
                "API_BASE_URL",
                "\"${project.findProperty("API_BASE_URL_DEV")}\"",
            )
        }
        release {
            isMinifyEnabled = false
            buildConfigField(
                "String",
                "API_BASE_URL",
                "\"${project.findProperty("API_BASE_URL_PROD")}\"",
            )
        }
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.service)
    implementation(libs.androidx.lifecycle.viewmodel.compose)

    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.graphics)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)

    implementation(libs.hilt.android)
    implementation(libs.hilt.navigation.compose)
    ksp(libs.hilt.compiler)

    implementation(libs.camerax.core)
    implementation(libs.camerax.camera2)
    implementation(libs.camerax.lifecycle)
    implementation(libs.camerax.view)
    implementation(libs.mlkit.barcode)

    implementation(libs.room.runtime)
    implementation(libs.room.ktx)
    ksp(libs.room.compiler)

    implementation(libs.retrofit)
    implementation(libs.retrofit.serialization)
    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.coroutines.play.services)

    implementation(libs.androidx.security.crypto)
    implementation(libs.play.services.location)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
}
