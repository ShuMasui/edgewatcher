package com.edgewatcher.di;

import com.edgewatcher.infrastructure.api.EdgeWatcherService;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;
import kotlinx.serialization.json.Json;
import okhttp3.OkHttpClient;

@ScopeMetadata("javax.inject.Singleton")
@QualifierMetadata
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class AppModule_ServiceFactory implements Factory<EdgeWatcherService> {
  private final Provider<OkHttpClient> clientProvider;

  private final Provider<Json> jsonProvider;

  private AppModule_ServiceFactory(Provider<OkHttpClient> clientProvider,
      Provider<Json> jsonProvider) {
    this.clientProvider = clientProvider;
    this.jsonProvider = jsonProvider;
  }

  @Override
  public EdgeWatcherService get() {
    return service(clientProvider.get(), jsonProvider.get());
  }

  public static AppModule_ServiceFactory create(Provider<OkHttpClient> clientProvider,
      Provider<Json> jsonProvider) {
    return new AppModule_ServiceFactory(clientProvider, jsonProvider);
  }

  public static EdgeWatcherService service(OkHttpClient client, Json json) {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.service(client, json));
  }
}
